# Account Hub — Фаза 2c-ii (Сессии UI + register/reset HTML) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Достроить аккаунт-хаб для Identity: страница управления сессиями (список/завершить/выйти везде) под hub-сессией + server-rendered HTML для регистрации, восстановления и подтверждения email.

**Architecture:** Хаб-страницы под `RequireHubSession` дёргают уже существующие use-cases (`ListSessions`/`TerminateSession`/`TerminateAllSessions`) in-process по subject из hub-контекста. Публичные HTML-страницы (signup/forgot/reset/verify) — новые хендлеры поверх существующих use-cases (`RegisterUser`/`RequestPasswordReset`/`ResetPassword`/`VerifyEmail`). Всё server-rendered (`html/template`, embed).

**Scope note:** 2c-ii — UI поверх готовой логики. Свитчер воркспейсов НЕ входит (нет Workspace-модуля — следующая фаза).

**Tech Stack:** Go 1.26, chi, html/template, testify. Module `github.com/denislibs/papirus-identity-center`.

---

## Предпосылки
Готово: use-cases `ListSessions`/`TerminateSession(userID,id)`/`TerminateAllSessions(userID)`, `RegisterUser`/`VerifyEmail`/`RequestPasswordReset`/`ResetPassword`, `GetProfile`. Hub: `RequireHubSession` + `HubUserIDFromContext`, `HubHandlers` (GET /account), `MustLoadTemplates()` (glob templates/*.html). Фейки в `presentation/http/fakes_test.go`: `fakeUsers`, `fakeSessions` (in-memory), `fakeHydra`, `fakeHubStore`.

---

## File Structure
```
internal/presentation/http/
  templates/sessions.html          страница сессий
  templates/signup.html            форма регистрации + результат
  templates/forgot_password.html   запрос сброса
  templates/reset_password.html    установка нового пароля
  templates/message.html           универсальная страница-сообщение (verify/успех)
  hub_handlers.go                  (+ методы sessions на HubHandlers)
  hub_handlers_test.go             (+ тесты)
  public_pages.go                  PublicPageHandlers (signup/forgot/reset/verify HTML)
  public_pages_test.go
internal/infrastructure/di/wire.go (+ провайдеры/аргументы)
internal/infrastructure/httpserver/server.go (+ mount)
```

---

## Task 1: Страница управления сессиями (hub)

**Files:**
- Create: `internal/presentation/http/templates/sessions.html`
- Modify: `internal/presentation/http/hub_handlers.go` (расширить HubHandlers)
- Test: `internal/presentation/http/hub_handlers_test.go` (добавить тесты)

**Логика (под RequireHubSession, user id из контекста):**
- `GET /account/sessions` — список активных сессий (ListSessions) → рендер `sessions.html` с формами завершения.
- `POST /account/sessions/{id}/terminate` — TerminateSession(userID, id) → 303 на `/account/sessions`.
- `POST /account/sessions/logout-all` — TerminateAllSessions(userID) → 303 на `/account/sessions`.

- [ ] **Step 1: Шаблон** — `internal/presentation/http/templates/sessions.html`:
```html
<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Sessions — Papyrus</title></head>
<body>
  <h1>Active sessions</h1>
  <ul>
    {{range .Sessions}}
    <li>
      <span>{{.DeviceName}} — {{.IP}}</span>
      <form method="post" action="/account/sessions/{{.ID}}/terminate" style="display:inline">
        <button type="submit">Terminate</button>
      </form>
    </li>
    {{else}}
    <li>No active sessions</li>
    {{end}}
  </ul>
  <form method="post" action="/account/sessions/logout-all">
    <button type="submit">Log out everywhere</button>
  </form>
  <p><a href="/account">Back to account</a></p>
</body>
</html>
```

- [ ] **Step 2: Падающий тест** — добавить в `internal/presentation/http/hub_handlers_test.go`:
```go
func TestSessionsPageListsAndTerminates(t *testing.T) {
	users := newFakeUsers()
	_ = users.Create(context.Background(), &domain.User{ID: "user-9", Email: "me@x.com", Name: "Me"})
	sessions := &fakeSessions{}
	_ = sessions.Create(context.Background(), &domain.Session{ID: "s1", UserID: "user-9", HydraSessionID: "sid1", DeviceName: "Chrome", IP: "1.2.3.4"})
	hydra := &fakeHydra{}
	store := &fakeHubStore{created: "user-9", id: "hubid-9"}

	h := apphttp.NewHubHandlers(
		appidentity.NewGetProfile(users),
		appidentity.NewListSessions(sessions),
		appidentity.NewTerminateSession(sessions, hydra),
		appidentity.NewTerminateAllSessions(sessions, hydra),
		apphttp.MustLoadTemplates(),
	)
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) {
		pr.Use(apphttp.RequireHubSession(store))
		h.Register(pr)
	})
	srv := httptest.NewServer(r)
	defer srv.Close()
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	// list
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/account/sessions", nil)
	req.AddCookie(&http.Cookie{Name: "hub_session", Value: "hubid-9"})
	resp, err := cl.Do(req)
	require.NoError(t, err)
	body := make([]byte, 8192); n, _ := resp.Body.Read(body); resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body[:n]), "Chrome")

	// terminate one
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/account/sessions/s1/terminate", nil)
	req2.AddCookie(&http.Cookie{Name: "hub_session", Value: "hubid-9"})
	resp2, err := cl.Do(req2)
	require.NoError(t, err); resp2.Body.Close()
	require.Equal(t, http.StatusSeeOther, resp2.StatusCode)
	require.Equal(t, "sid1", hydra.revokedSID)
}

func TestSessionsLogoutAll(t *testing.T) {
	users := newFakeUsers()
	sessions := &fakeSessions{}
	hydra := &fakeHydra{}
	store := &fakeHubStore{created: "user-9", id: "hubid-9"}
	h := apphttp.NewHubHandlers(
		appidentity.NewGetProfile(users), appidentity.NewListSessions(sessions),
		appidentity.NewTerminateSession(sessions, hydra), appidentity.NewTerminateAllSessions(sessions, hydra),
		apphttp.MustLoadTemplates(),
	)
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) { pr.Use(apphttp.RequireHubSession(store)); h.Register(pr) })
	srv := httptest.NewServer(r); defer srv.Close()
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/account/sessions/logout-all", nil)
	req.AddCookie(&http.Cookie{Name: "hub_session", Value: "hubid-9"})
	resp, err := cl.Do(req); require.NoError(t, err); resp.Body.Close()
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "user-9", hydra.revokedSubject)
}
```
(The existing `TestAccountPageRendersProfile` must be updated to the new `NewHubHandlers` signature — see Step 4.)

- [ ] **Step 3: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -run "TestSessionsPage|TestSessionsLogoutAll" -v` → FAIL (сигнатура/методы).

- [ ] **Step 4: Реализовать** — переписать `internal/presentation/http/hub_handlers.go`:
```go
package http

import (
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"

	appidentity "github.com/denislibs/papirus-identity-center/internal/application/identity"
)

// HubHandlers render the authenticated account hub pages.
type HubHandlers struct {
	profile      *appidentity.GetProfile
	listSessions *appidentity.ListSessions
	terminate    *appidentity.TerminateSession
	terminateAll *appidentity.TerminateAllSessions
	tpl          *template.Template
}

func NewHubHandlers(profile *appidentity.GetProfile, list *appidentity.ListSessions,
	terminate *appidentity.TerminateSession, terminateAll *appidentity.TerminateAllSessions,
	tpl *template.Template) *HubHandlers {
	return &HubHandlers{profile: profile, listSessions: list, terminate: terminate, terminateAll: terminateAll, tpl: tpl}
}

func (h *HubHandlers) Register(r chi.Router) {
	r.Get("/account", h.account)
	r.Get("/account/sessions", h.sessions)
	r.Post("/account/sessions/{id}/terminate", h.terminateSession)
	r.Post("/account/sessions/logout-all", h.logoutAll)
}

func (h *HubHandlers) account(w http.ResponseWriter, r *http.Request) {
	userID := HubUserIDFromContext(r.Context())
	u, err := h.profile.Execute(r.Context(), userID)
	if err != nil {
		http.Error(w, "profile error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.tpl.ExecuteTemplate(w, "account.html", map[string]any{"Email": u.Email, "Name": u.Name})
}

func (h *HubHandlers) sessions(w http.ResponseWriter, r *http.Request) {
	userID := HubUserIDFromContext(r.Context())
	list, err := h.listSessions.Execute(r.Context(), userID)
	if err != nil {
		http.Error(w, "sessions error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.tpl.ExecuteTemplate(w, "sessions.html", map[string]any{"Sessions": list})
}

func (h *HubHandlers) terminateSession(w http.ResponseWriter, r *http.Request) {
	userID := HubUserIDFromContext(r.Context())
	id := chi.URLParam(r, "id")
	_ = h.terminate.Execute(r.Context(), userID, id) // ownership enforced in use-case; ignore not-found on redirect
	http.Redirect(w, r, "/account/sessions", http.StatusSeeOther)
}

func (h *HubHandlers) logoutAll(w http.ResponseWriter, r *http.Request) {
	userID := HubUserIDFromContext(r.Context())
	_ = h.terminateAll.Execute(r.Context(), userID)
	http.Redirect(w, r, "/account/sessions", http.StatusSeeOther)
}
```
Also update `TestAccountPageRendersProfile` in `hub_handlers_test.go` to the new `NewHubHandlers(NewGetProfile(users), NewListSessions(sessions), NewTerminateSession(sessions, hydra), NewTerminateAllSessions(sessions, hydra), MustLoadTemplates())` signature (construct a `&fakeSessions{}` and `&fakeHydra{}` as needed).

- [ ] **Step 5: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -run "TestAccountPage|TestSessions" -v` → PASS.

- [ ] **Step 6: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/presentation/http/hub_handlers.go internal/presentation/http/hub_handlers_test.go internal/presentation/http/templates/sessions.html
git commit -m "feat(hub): sessions management page (list/terminate/logout-all)"
```

---

## Task 2: Публичные HTML-страницы (signup / forgot / reset / verify)

**Files:**
- Create: `internal/presentation/http/templates/{signup,forgot_password,reset_password,message}.html`
- Create: `internal/presentation/http/public_pages.go`
- Test: `internal/presentation/http/public_pages_test.go`

**Роуты (публичные):**
- `GET /signup` форма → `POST /signup` (RegisterUser) → `message.html` («проверьте почту»).
- `GET /forgot-password` форма → `POST /forgot-password` (RequestPasswordReset) → `message.html` («если аккаунт есть — письмо отправлено»).
- `GET /reset-password?token=` форма → `POST /reset-password` (ResetPassword) → `message.html` («пароль изменён») или ошибка.
- `GET /verify-email?token=` (HTML-версия) → VerifyEmail → `message.html` («email подтверждён»).

- [ ] **Step 1: Шаблоны**

`templates/message.html`:
```html
<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>{{.Title}} — Papyrus</title></head>
<body><h1>{{.Title}}</h1><p>{{.Message}}</p><p><a href="/account">Continue</a></p></body></html>
```
`templates/signup.html`:
```html
<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Sign up — Papyrus</title></head>
<body><h1>Create account</h1>
{{if .Error}}<p role="alert">{{.Error}}</p>{{end}}
<form method="post" action="/signup">
  <label>Name <input name="name"></label>
  <label>Email <input type="email" name="email" required></label>
  <label>Password <input type="password" name="password" required></label>
  <button type="submit">Sign up</button>
</form>
<p><a href="/auth/login">Already have an account? Log in</a></p></body></html>
```
`templates/forgot_password.html`:
```html
<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Forgot password — Papyrus</title></head>
<body><h1>Reset your password</h1>
<form method="post" action="/forgot-password">
  <label>Email <input type="email" name="email" required></label>
  <button type="submit">Send reset link</button>
</form></body></html>
```
`templates/reset_password.html`:
```html
<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Set new password — Papyrus</title></head>
<body><h1>Set a new password</h1>
{{if .Error}}<p role="alert">{{.Error}}</p>{{end}}
<form method="post" action="/reset-password">
  <input type="hidden" name="token" value="{{.Token}}">
  <label>New password <input type="password" name="password" required></label>
  <button type="submit">Change password</button>
</form></body></html>
```

- [ ] **Step 2: Падающий тест** — `internal/presentation/http/public_pages_test.go`:
```go
package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	appidentity "github.com/denislibs/papirus-identity-center/internal/application/identity"
	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

func buildPublicPages(t *testing.T) (*httptest.Server, *fakeUsers, *fakeTokens, *fakeMailer) {
	t.Helper()
	users := newFakeUsers()
	tokens := newFakeTokens()
	mailer := newFakeMailer()
	base := "https://acc.example"
	h := apphttp.NewPublicPageHandlers(
		appidentity.NewRegisterUser(users, fakeHasher{}, tokens, mailer, base),
		appidentity.NewVerifyEmail(users, tokens),
		appidentity.NewRequestPasswordReset(users, tokens, mailer, base),
		appidentity.NewResetPassword(users, fakeHasher{}, tokens),
		apphttp.MustLoadTemplates(),
	)
	r := chi.NewRouter()
	h.Register(r)
	return httptest.NewServer(r), users, tokens, mailer
}

func getBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b := make([]byte, 8192); n, _ := resp.Body.Read(b); resp.Body.Close()
	return string(b[:n])
}

func TestSignupGETShowsForm(t *testing.T) {
	srv, _, _, _ := buildPublicPages(t); defer srv.Close()
	resp, err := http.Get(srv.URL + "/signup")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, getBody(t, resp), `action="/signup"`)
}

func TestSignupPOSTRegistersAndSendsMail(t *testing.T) {
	srv, _, _, mailer := buildPublicPages(t); defer srv.Close()
	form := url.Values{"email": {"new@x.com"}, "password": {"long-enough-pw"}, "name": {"New"}}
	resp, err := http.PostForm(srv.URL+"/signup", form)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, mailer.verifications, 1)
	require.Contains(t, getBody(t, resp), "email") // "check your email" message
}

func TestSignupPOSTWeakPasswordShowsError(t *testing.T) {
	srv, _, _, _ := buildPublicPages(t); defer srv.Close()
	form := url.Values{"email": {"x@x.com"}, "password": {"short"}}
	resp, err := http.PostForm(srv.URL+"/signup", form)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode) // re-render form with error
	require.Contains(t, strings.ToLower(getBody(t, resp)), "password")
}

func TestVerifyEmailPageMarksVerified(t *testing.T) {
	srv, users, tokens, _ := buildPublicPages(t); defer srv.Close()
	_ = users.Create(context.Background(), &domain.User{ID: "u1", Email: "a@x.com"})
	tok, _ := tokens.Issue(context.Background(), domain.PurposeVerifyEmail, "u1", 0)
	resp, err := http.Get(srv.URL + "/verify-email?token=" + tok)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	got, _ := users.FindByID(context.Background(), "u1")
	require.True(t, got.EmailVerified)
}

func TestResetPasswordFlow(t *testing.T) {
	srv, users, tokens, _ := buildPublicPages(t); defer srv.Close()
	_ = users.Create(context.Background(), &domain.User{ID: "u1", Email: "a@x.com", PasswordHash: "old"})
	tok, _ := tokens.Issue(context.Background(), domain.PurposePasswordReset, "u1", 0)

	// GET form shows token
	resp, err := http.Get(srv.URL + "/reset-password?token=" + tok)
	require.NoError(t, err)
	require.Contains(t, getBody(t, resp), tok)

	// POST new password
	form := url.Values{"token": {tok}, "password": {"brand-new-pw"}}
	resp2, err := http.PostForm(srv.URL+"/reset-password", form)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	got, _ := users.FindByID(context.Background(), "u1")
	require.Equal(t, "hashed:brand-new-pw", got.PasswordHash)
}
```
(Reuses `fakeUsers`/`fakeHasher`/`fakeTokens`/`fakeMailer` from the existing `fakes_test.go`.)

- [ ] **Step 3: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -run "TestSignup|TestVerifyEmailPage|TestResetPasswordFlow" -v` → FAIL (нет NewPublicPageHandlers).

- [ ] **Step 4: Реализовать** — `internal/presentation/http/public_pages.go`:
```go
package http

import (
	"errors"
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"

	appidentity "github.com/denislibs/papirus-identity-center/internal/application/identity"
	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

// PublicPageHandlers render the public (unauthenticated) auth pages.
type PublicPageHandlers struct {
	register *appidentity.RegisterUser
	verify   *appidentity.VerifyEmail
	reqReset *appidentity.RequestPasswordReset
	reset    *appidentity.ResetPassword
	tpl      *template.Template
}

func NewPublicPageHandlers(register *appidentity.RegisterUser, verify *appidentity.VerifyEmail,
	reqReset *appidentity.RequestPasswordReset, reset *appidentity.ResetPassword,
	tpl *template.Template) *PublicPageHandlers {
	return &PublicPageHandlers{register: register, verify: verify, reqReset: reqReset, reset: reset, tpl: tpl}
}

func (h *PublicPageHandlers) Register(r chi.Router) {
	r.Get("/signup", h.signupForm)
	r.Post("/signup", h.signupSubmit)
	r.Get("/forgot-password", h.forgotForm)
	r.Post("/forgot-password", h.forgotSubmit)
	r.Get("/reset-password", h.resetForm)
	r.Post("/reset-password", h.resetSubmit)
	r.Get("/verify-email", h.verifyEmail)
}

func (h *PublicPageHandlers) render(w http.ResponseWriter, name string, data any, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = h.tpl.ExecuteTemplate(w, name, data)
}

func (h *PublicPageHandlers) msg(w http.ResponseWriter, title, message string) {
	h.render(w, "message.html", map[string]any{"Title": title, "Message": message}, http.StatusOK)
}

func (h *PublicPageHandlers) signupForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "signup.html", map[string]any{"Error": ""}, http.StatusOK)
}

func (h *PublicPageHandlers) signupSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	_, err := h.register.Execute(r.Context(), appidentity.RegisterInput{
		Email: r.PostForm.Get("email"), Password: r.PostForm.Get("password"), Name: r.PostForm.Get("name"),
	})
	if err != nil {
		h.render(w, "signup.html", map[string]any{"Error": humanizeAuthError(err)}, http.StatusOK)
		return
	}
	h.msg(w, "Check your email", "We sent a verification link to your email address.")
}

func (h *PublicPageHandlers) forgotForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "forgot_password.html", nil, http.StatusOK)
}

func (h *PublicPageHandlers) forgotSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	_ = h.reqReset.Execute(r.Context(), r.PostForm.Get("email")) // silent regardless
	h.msg(w, "Check your email", "If an account exists for that email, a reset link has been sent.")
}

func (h *PublicPageHandlers) resetForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "reset_password.html", map[string]any{"Token": r.URL.Query().Get("token"), "Error": ""}, http.StatusOK)
}

func (h *PublicPageHandlers) resetSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	token := r.PostForm.Get("token")
	if err := h.reset.Execute(r.Context(), token, r.PostForm.Get("password")); err != nil {
		h.render(w, "reset_password.html", map[string]any{"Token": token, "Error": humanizeAuthError(err)}, http.StatusOK)
		return
	}
	h.msg(w, "Password changed", "Your password has been updated. You can now log in.")
}

func (h *PublicPageHandlers) verifyEmail(w http.ResponseWriter, r *http.Request) {
	if err := h.verify.Execute(r.Context(), r.URL.Query().Get("token")); err != nil {
		h.msg(w, "Verification failed", "This link is invalid or has expired.")
		return
	}
	h.msg(w, "Email verified", "Your email has been verified. You can now log in.")
}

func humanizeAuthError(err error) string {
	switch {
	case errors.Is(err, domain.ErrUserExists):
		return "An account with this email already exists."
	case errors.Is(err, domain.ErrWeakPassword):
		return "Password is too weak (min 8 characters)."
	case errors.Is(err, domain.ErrInvalidEmail):
		return "Please enter a valid email."
	case errors.Is(err, domain.ErrTokenInvalid):
		return "This link is invalid or has expired."
	default:
		return "Something went wrong. Please try again."
	}
}
```
**NOTE (route conflict):** the JSON `IdentityHandlers` already register `GET /verify-email` and `POST /password-reset/*` and `POST /register`. This PublicPageHandlers registers `GET /verify-email` too. To avoid a chi duplicate-route panic, in Task 3 the JSON `IdentityHandlers.Register` MUST drop its own `GET /verify-email` route (the HTML page replaces it; email links point to `/verify-email`). Keep the JSON `POST /register`, `POST /password-reset/request`, `POST /password-reset/confirm` (no path clash with the new HTML routes `/signup`, `/forgot-password`, `/reset-password`). Update `IdentityHandlers.Register` accordingly and adjust its test if it asserted the verify route.

- [ ] **Step 5: Убрать дублирующий GET /verify-email из JSON-хендлеров** — в `internal/presentation/http/identity_handlers.go` в методе `Register` удалить строку `r.Get("/verify-email", h.handleVerifyEmail)` (и, если хочешь, приватный `handleVerifyEmail` — оставить нельзя неиспользуемым: удалить метод тоже). Поправить `identity_handlers_test.go`, если там тестировался `/verify-email` через JSON (заменить/удалить тот кейс).

- [ ] **Step 6: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -v` → PASS (все).

- [ ] **Step 7: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/presentation/http/public_pages.go internal/presentation/http/public_pages_test.go internal/presentation/http/identity_handlers.go internal/presentation/http/identity_handlers_test.go internal/presentation/http/templates/
git commit -m "feat(hub): public signup/forgot/reset/verify HTML pages"
```

---

## Task 3: DI-проводка + mount

**Files:**
- Modify: `internal/infrastructure/di/wire.go` (+ regen)
- Modify: `internal/infrastructure/httpserver/server.go`

- [ ] **Step 1: Провайдеры** — в `wire.go`:
  - обновить `provideHubHandlers`, чтобы он строил все 4 use-case и передавал в `NewHubHandlers`:
```go
func provideHubHandlers(users domainidentity.UserRepository, sessions domainidentity.SessionRepository,
	hydraClient domainidentity.HydraClient) *apphttp.HubHandlers {
	return apphttp.NewHubHandlers(
		appidentity.NewGetProfile(users),
		appidentity.NewListSessions(sessions),
		appidentity.NewTerminateSession(sessions, hydraClient),
		appidentity.NewTerminateAllSessions(sessions, hydraClient),
		apphttp.MustLoadTemplates(),
	)
}
```
  - добавить `providePublicPages`:
```go
func providePublicPages(cfg config.Config, users domainidentity.UserRepository,
	hasher domainidentity.PasswordHasher, tokens domainidentity.VerificationTokens,
	mailer domainidentity.Mailer) *apphttp.PublicPageHandlers {
	return apphttp.NewPublicPageHandlers(
		appidentity.NewRegisterUser(users, hasher, tokens, mailer, cfg.BaseURL),
		appidentity.NewVerifyEmail(users, tokens),
		appidentity.NewRequestPasswordReset(users, tokens, mailer, cfg.BaseURL),
		appidentity.NewResetPassword(users, hasher, tokens),
		apphttp.MustLoadTemplates(),
	)
}
```
  - обновить `provideServer`, добавив параметр `public *apphttp.PublicPageHandlers`; добавить `providePublicPages` в `wire.Build`.

- [ ] **Step 2: Mount** — в `internal/infrastructure/httpserver/server.go` расширить `NewRouter` параметром `public *apphttp.PublicPageHandlers` и добавить `public.Register(r)` (публично, до групп). Обновить `server_test.go`: добавить `apphttp.NewPublicPageHandlers(nil,nil,nil,nil, apphttp.MustLoadTemplates())` в вызов (только `/healthz` тестируется, nil use-cases не вызываются).

- [ ] **Step 3: Regen + build + test.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && make wire && go build ./... && go vet ./... && go test -short ./...` → всё чисто/зелёно.

- [ ] **Step 4: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/infrastructure/di/ internal/infrastructure/httpserver/
git commit -m "feat(hub): wire sessions page + public auth pages"
```

---

## Task 4: Финальная проверка + E2E smoke

- [ ] **Step 1:** `cd /Users/denisurevic/Downloads/ББД/platform && go test ./... && go vet ./... && go build ./...` → всё зелёно/чисто.

- [ ] **Step 2: E2E smoke (Docker).** `docker compose up -d --build --wait`. Проверить публичные страницы без авторизации:
```bash
curl -sf http://localhost:8090/signup | grep -q 'action="/signup"' && echo "signup form OK"
curl -sf http://localhost:8090/forgot-password | grep -q 'Send reset link' && echo "forgot form OK"
```
Зарегистрировать через HTML-форму и проверить, что письмо «ушло» (LogMailer в логах):
```bash
curl -sf -X POST http://localhost:8090/signup --data-urlencode 'email=htmluser@example.com' --data-urlencode 'password=long-enough-pw' --data-urlencode 'name=HTML' | grep -qi 'email' && echo "signup submit OK"
docker compose logs platform-core | grep 'htmluser@example.com'  # verification link
```
Затем `docker compose down`.

- [ ] **Step 3: Push.** `cd /Users/denisurevic/Downloads/ББД/platform && git push origin main`.

---

## Definition of Done (Фаза 2c-ii)
- `/account/sessions` (под hub-сессией): список сессий, завершить одну (ownership), выйти везде.
- Публичные HTML: `/signup`, `/forgot-password`, `/reset-password`, `/verify-email` — поверх существующих use-cases; ошибки по-человечески.
- JSON `GET /verify-email` заменён HTML-страницей (email-ссылки ведут на неё); JSON `POST`-эндпоинты сохранены.
- Все тесты зелёные; vet/build чисто; E2E-smoke публичных страниц ок. Запушено.

## Следующая фаза
Workspace-модуль: воркспейсы, участники+приглашения, оргструктура (подразделения/должности), включение продуктов + REST API для продуктов; затем свитчер воркспейсов в хабе.
