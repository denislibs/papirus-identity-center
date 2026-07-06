# Workspace модуль — W4 (UI в хабе) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Server-rendered UI воркспейсов в аккаунт-хабе: список/создание воркспейсов (свитчер) и страница воркспейса с управлением участниками, оргструктурой и продуктами — под hub-сессией, поверх готовых use-cases.

**Architecture:** Новый `HubWorkspaceHandlers` (package `http`) под `RequireHubSession`; текущий пользователь = `HubUserIDFromContext`. Вызывает workspace use-cases in-process. HTML через `html/template` (embed, auto-escape). Роуты под `/account/workspaces...` (не конфликтуют с Bearer JSON API `/workspaces`).

**Scope note (W4):** UI поверх W1–W3. Никакой новой бизнес-логики.

**Tech Stack:** Go 1.26, chi, html/template, testify. Module `github.com/denislibs/papirus-identity-center`.

---

## File Structure
```
internal/presentation/http/
  templates/workspaces.html         список + форма создания
  templates/workspace_detail.html   участники/структура/продукты + формы управления
  hub_workspace_handlers.go         HubWorkspaceHandlers + routes + methods
  hub_workspace_handlers_test.go
internal/infrastructure/di/wire.go  (+ провайдер + mount под RequireHubSession)
internal/infrastructure/httpserver/server.go (+ параметр + mount)
```

---

## Task 1: Страница списка/создания воркспейсов (свитчер)

**Files:** `templates/workspaces.html`, `hub_workspace_handlers.go`, `hub_workspace_handlers_test.go`

**Роуты (под RequireHubSession):**
- `GET /account/workspaces` → список моих воркспейсов (ListMyWorkspaces) + форма создания.
- `POST /account/workspaces` {name} → CreateWorkspace(hubUser, name) → 303 на `/account/workspaces/{id}`.

- [ ] **Step 1: Шаблон** — `templates/workspaces.html`:
```html
<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Workspaces — Papyrus</title></head>
<body>
  <h1>Your workspaces</h1>
  <ul>
    {{range .Workspaces}}
    <li><a href="/account/workspaces/{{.ID}}">{{.Name}}</a> <small>{{.Slug}}</small></li>
    {{else}}
    <li>No workspaces yet</li>
    {{end}}
  </ul>
  <h2>Create a workspace</h2>
  <form method="post" action="/account/workspaces">
    <label>Name <input name="name" required></label>
    <button type="submit">Create</button>
  </form>
  <p><a href="/account">Back to account</a></p>
</body>
</html>
```

- [ ] **Step 2: Падающий тест** — `hub_workspace_handlers_test.go`:
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

	appws "github.com/denislibs/papirus-identity-center/internal/application/workspace"
	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

// buildHubWS wires HubWorkspaceHandlers with the workspace http-fakes behind RequireHubSession(user).
func buildHubWS(t *testing.T, userID string) (*httptest.Server, *fakeMembersHTTP) {
	t.Helper()
	members := newFakeMembersHTTP()
	ws := newFakeWSHTTP(members)
	invites := newFakeInvitesHTTP()
	units := &fakeOrgUnitsHTTP{}
	positions := &fakePositionsHTTP{}
	products := &fakeProductsHTTP{list: nil}
	wprods := newFakeWorkspaceProductsHTTP()
	mailer := &fakeWSMailer{}
	store := &fakeHubStore{created: userID, id: "hubid-ws"}

	h := apphttp.NewHubWorkspaceHandlers(
		appws.NewCreateWorkspace(ws, members),
		appws.NewListMyWorkspaces(ws),
		appws.NewListMembers(members),
		appws.NewInviteMember(ws, members, invites, mailer, "https://acc.example"),
		appws.NewCreateOrgUnit(members, units),
		appws.NewListOrgUnits(members, units),
		appws.NewCreatePosition(members, positions),
		appws.NewListPositions(members, positions),
		appws.NewListProducts(products),
		appws.NewEnableProduct(members, products, wprods),
		appws.NewListEnabledProducts(members, wprods),
		apphttp.MustLoadTemplates(),
	)
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) { pr.Use(apphttp.RequireHubSession(store)); h.Register(pr) })
	return httptest.NewServer(r), members
}

func noRedir() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func hubReq(t *testing.T, method, url string, body string) *http.Request {
	t.Helper()
	var r *http.Request
	if body == "" {
		r, _ = http.NewRequest(method, url, nil)
	} else {
		r, _ = http.NewRequest(method, url, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	r.AddCookie(&http.Cookie{Name: "hub_session", Value: "hubid-ws"})
	return r
}

func TestHubWorkspacesListAndCreate(t *testing.T) {
	srv, _ := buildHubWS(t, "user-1")
	defer srv.Close()

	// initially empty list renders
	resp, err := noRedir().Do(hubReq(t, http.MethodGet, srv.URL+"/account/workspaces", ""))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// create → 303 to detail
	form := url.Values{"name": {"Acme"}}.Encode()
	resp2, err := noRedir().Do(hubReq(t, http.MethodPost, srv.URL+"/account/workspaces", form))
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp2.StatusCode)
	require.True(t, strings.HasPrefix(resp2.Header.Get("Location"), "/account/workspaces/"))
	resp2.Body.Close()

	// list now shows Acme
	resp3, err := noRedir().Do(hubReq(t, http.MethodGet, srv.URL+"/account/workspaces", ""))
	require.NoError(t, err)
	b := make([]byte, 8192); n, _ := resp3.Body.Read(b); resp3.Body.Close()
	require.Contains(t, string(b[:n]), "Acme")
}

var _ = context.Background
```

- [ ] **Step 3: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -run TestHubWorkspaces -v` → FAIL (нет NewHubWorkspaceHandlers).

- [ ] **Step 4: Реализовать** — `hub_workspace_handlers.go` (только list+create в этой задаче; detail-методы добавит Task 2, но конструктор сразу принимает все use-cases):
```go
package http

import (
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"

	appws "github.com/denislibs/papirus-identity-center/internal/application/workspace"
)

// HubWorkspaceHandlers renders workspace management pages in the account hub.
type HubWorkspaceHandlers struct {
	create        *appws.CreateWorkspace
	listMine      *appws.ListMyWorkspaces
	listMembers   *appws.ListMembers
	invite        *appws.InviteMember
	createUnit    *appws.CreateOrgUnit
	listUnits     *appws.ListOrgUnits
	createPos     *appws.CreatePosition
	listPos       *appws.ListPositions
	listProducts  *appws.ListProducts
	enableProduct *appws.EnableProduct
	listEnabled   *appws.ListEnabledProducts
	tpl           *template.Template
}

func NewHubWorkspaceHandlers(create *appws.CreateWorkspace, listMine *appws.ListMyWorkspaces,
	listMembers *appws.ListMembers, invite *appws.InviteMember, createUnit *appws.CreateOrgUnit,
	listUnits *appws.ListOrgUnits, createPos *appws.CreatePosition, listPos *appws.ListPositions,
	listProducts *appws.ListProducts, enableProduct *appws.EnableProduct, listEnabled *appws.ListEnabledProducts,
	tpl *template.Template) *HubWorkspaceHandlers {
	return &HubWorkspaceHandlers{
		create: create, listMine: listMine, listMembers: listMembers, invite: invite,
		createUnit: createUnit, listUnits: listUnits, createPos: createPos, listPos: listPos,
		listProducts: listProducts, enableProduct: enableProduct, listEnabled: listEnabled, tpl: tpl,
	}
}

func (h *HubWorkspaceHandlers) Register(r chi.Router) {
	r.Get("/account/workspaces", h.list)
	r.Post("/account/workspaces", h.createWorkspace)
	// detail + management routes added in Task 2
	h.registerDetail(r)
}

func (h *HubWorkspaceHandlers) list(w http.ResponseWriter, r *http.Request) {
	userID := HubUserIDFromContext(r.Context())
	list, err := h.listMine.Execute(r.Context(), userID)
	if err != nil {
		http.Error(w, "workspaces error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.tpl.ExecuteTemplate(w, "workspaces.html", map[string]any{"Workspaces": list})
}

func (h *HubWorkspaceHandlers) createWorkspace(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	userID := HubUserIDFromContext(r.Context())
	ws, err := h.create.Execute(r.Context(), userID, r.PostForm.Get("name"))
	if err != nil {
		http.Error(w, "could not create workspace", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/account/workspaces/"+ws.ID, http.StatusSeeOther)
}
```
And a temporary stub for `registerDetail` in a SEPARATE file `hub_workspace_detail.go` so Task 1 compiles (Task 2 fills it in):
```go
package http

import "github.com/go-chi/chi/v5"

// registerDetail mounts the workspace detail + management routes (implemented in Task 2).
func (h *HubWorkspaceHandlers) registerDetail(r chi.Router) {}
```

- [ ] **Step 5: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -run TestHubWorkspaces -v` → PASS.
(If http fakes `newFakeWorkspaceProductsHTTP`/`fakeProductsHTTP`/`fakeOrgUnitsHTTP`/`fakePositionsHTTP` don't yet expose the exact constructors used in the test, adjust the test's fake construction to match what exists in `workspace_fakes_test.go` — do NOT change production code for it.)

- [ ] **Step 6: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/presentation/http/hub_workspace_handlers.go internal/presentation/http/hub_workspace_detail.go internal/presentation/http/hub_workspace_handlers_test.go internal/presentation/http/templates/workspaces.html
git commit -m "feat(hub): workspaces list + create page (switcher)"
```

---

## Task 2: Страница воркспейса + формы управления

**Files:** `templates/workspace_detail.html`, `hub_workspace_detail.go` (заменить стаб), `hub_workspace_handlers_test.go` (+ тесты)

**Роуты (под RequireHubSession):**
- `GET /account/workspaces/{id}` → детали: участники, подразделения, должности, включённые продукты, реестр продуктов + формы.
- `POST /account/workspaces/{id}/invites` {email, role} → InviteMember → 303 назад.
- `POST /account/workspaces/{id}/org-units` {name} → CreateOrgUnit → 303 назад.
- `POST /account/workspaces/{id}/positions` {title} → CreatePosition → 303 назад.
- `POST /account/workspaces/{id}/products` {product_key} → EnableProduct → 303 назад.

- [ ] **Step 1: Шаблон** — `templates/workspace_detail.html`:
```html
<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Workspace — Papyrus</title></head>
<body>
  <p><a href="/account/workspaces">← Workspaces</a></p>

  <h2>Members</h2>
  <ul>{{range .Members}}<li>{{.UserID}} — {{.Role}} ({{.Status}})</li>{{end}}</ul>
  <form method="post" action="/account/workspaces/{{.WorkspaceID}}/invites">
    <input type="email" name="email" placeholder="email" required>
    <select name="role"><option value="member">member</option><option value="admin">admin</option></select>
    <button type="submit">Invite</button>
  </form>

  <h2>Departments</h2>
  <ul>{{range .Units}}<li>{{.Name}}</li>{{end}}</ul>
  <form method="post" action="/account/workspaces/{{.WorkspaceID}}/org-units">
    <input name="name" placeholder="department" required><button type="submit">Add</button>
  </form>

  <h2>Positions</h2>
  <ul>{{range .Positions}}<li>{{.Title}}</li>{{end}}</ul>
  <form method="post" action="/account/workspaces/{{.WorkspaceID}}/positions">
    <input name="title" placeholder="position" required><button type="submit">Add</button>
  </form>

  <h2>Products</h2>
  <ul>{{range .Enabled}}<li>{{.Name}} (enabled)</li>{{end}}</ul>
  <form method="post" action="/account/workspaces/{{.WorkspaceID}}/products">
    <select name="product_key">{{range .Registry}}<option value="{{.Key}}">{{.Name}}</option>{{end}}</select>
    <button type="submit">Enable</button>
  </form>
</body>
</html>
```

- [ ] **Step 2: Падающий тест** — добавить в `hub_workspace_handlers_test.go`:
```go
func TestHubWorkspaceDetailAndManage(t *testing.T) {
	srv, _ := buildHubWS(t, "owner-x")
	defer srv.Close()

	// create a workspace (owner-x becomes owner)
	form := url.Values{"name": {"Detail Co"}}.Encode()
	resp, err := noRedir().Do(hubReq(t, http.MethodPost, srv.URL+"/account/workspaces", form))
	require.NoError(t, err)
	loc := resp.Header.Get("Location"); resp.Body.Close()
	require.True(t, strings.HasPrefix(loc, "/account/workspaces/"))

	// detail renders (owner is a member)
	resp2, err := noRedir().Do(hubReq(t, http.MethodGet, srv.URL+loc, ""))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	b := make([]byte, 16384); n, _ := resp2.Body.Read(b); resp2.Body.Close()
	require.Contains(t, string(b[:n]), "Members")

	// add a department → 303 back
	dept := url.Values{"name": {"Sales"}}.Encode()
	resp3, err := noRedir().Do(hubReq(t, http.MethodPost, srv.URL+loc+"/org-units", dept))
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp3.StatusCode)
	resp3.Body.Close()

	// detail now shows Sales
	resp4, err := noRedir().Do(hubReq(t, http.MethodGet, srv.URL+loc, ""))
	require.NoError(t, err)
	b2 := make([]byte, 16384); n2, _ := resp4.Body.Read(b2); resp4.Body.Close()
	require.Contains(t, string(b2[:n2]), "Sales")
}
```

- [ ] **Step 3: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -run TestHubWorkspaceDetail -v` → FAIL (detail-роут не смонтирован).

- [ ] **Step 4: Реализовать** — заменить `hub_workspace_detail.go`:
```go
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (h *HubWorkspaceHandlers) registerDetail(r chi.Router) {
	r.Get("/account/workspaces/{id}", h.detail)
	r.Post("/account/workspaces/{id}/invites", h.inviteMember)
	r.Post("/account/workspaces/{id}/org-units", h.addUnit)
	r.Post("/account/workspaces/{id}/positions", h.addPosition)
	r.Post("/account/workspaces/{id}/products", h.enableProd)
}

func (h *HubWorkspaceHandlers) detail(w http.ResponseWriter, r *http.Request) {
	userID := HubUserIDFromContext(r.Context())
	wsID := chi.URLParam(r, "id")

	members, err := h.listMembers.Execute(r.Context(), wsID, userID)
	if err != nil {
		// not a member (or gone) → back to list
		http.Redirect(w, r, "/account/workspaces", http.StatusSeeOther)
		return
	}
	units, _ := h.listUnits.Execute(r.Context(), wsID, userID)
	positions, _ := h.listPos.Execute(r.Context(), wsID, userID)
	enabled, _ := h.listEnabled.Execute(r.Context(), wsID, userID)
	registry, _ := h.listProducts.Execute(r.Context())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.tpl.ExecuteTemplate(w, "workspace_detail.html", map[string]any{
		"WorkspaceID": wsID, "Members": members, "Units": units,
		"Positions": positions, "Enabled": enabled, "Registry": registry,
	})
}

func (h *HubWorkspaceHandlers) inviteMember(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	wsID := chi.URLParam(r, "id")
	_ = h.invite.Execute(r.Context(), wsID, HubUserIDFromContext(r.Context()), r.PostForm.Get("email"), r.PostForm.Get("role"))
	http.Redirect(w, r, "/account/workspaces/"+wsID, http.StatusSeeOther)
}

func (h *HubWorkspaceHandlers) addUnit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	wsID := chi.URLParam(r, "id")
	_, _ = h.createUnit.Execute(r.Context(), wsID, HubUserIDFromContext(r.Context()), r.PostForm.Get("name"), nil)
	http.Redirect(w, r, "/account/workspaces/"+wsID, http.StatusSeeOther)
}

func (h *HubWorkspaceHandlers) addPosition(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	wsID := chi.URLParam(r, "id")
	_, _ = h.createPos.Execute(r.Context(), wsID, HubUserIDFromContext(r.Context()), r.PostForm.Get("title"))
	http.Redirect(w, r, "/account/workspaces/"+wsID, http.StatusSeeOther)
}

func (h *HubWorkspaceHandlers) enableProd(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	wsID := chi.URLParam(r, "id")
	_ = h.enableProduct.Execute(r.Context(), wsID, HubUserIDFromContext(r.Context()), r.PostForm.Get("product_key"))
	http.Redirect(w, r, "/account/workspaces/"+wsID, http.StatusSeeOther)
}
```
(NB: management POST handlers ignore use-case errors and redirect back — the page re-renders current state; validation errors simply produce no change. Acceptable for MVP UI. Authorization is still enforced inside the use-cases: a non-owner/admin POST is a no-op.)

- [ ] **Step 5: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -run TestHubWorkspace -v` → PASS. Also full http suite: `go test ./internal/presentation/http/ -v`.

- [ ] **Step 6: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/presentation/http/hub_workspace_detail.go internal/presentation/http/hub_workspace_handlers_test.go internal/presentation/http/templates/workspace_detail.html
git commit -m "feat(hub): workspace detail + management forms"
```

---

## Task 3: DI + mount под RequireHubSession + ссылка из /account

**Files:** `internal/infrastructure/di/wire.go` (+ regen), `internal/infrastructure/httpserver/server.go`, `templates/account.html`

- [ ] **Step 1: Провайдер** — в `wire.go`:
```go
func provideHubWorkspaceHandlers(cfg config.Config, ws domainws.WorkspaceRepository, mem domainws.MemberRepository,
	inv domainws.InviteRepository, units domainws.OrgUnitRepository, positions domainws.PositionRepository,
	products domainws.ProductRepository, wp domainws.WorkspaceProductRepository, mailer domainws.WorkspaceMailer) *apphttp.HubWorkspaceHandlers {
	return apphttp.NewHubWorkspaceHandlers(
		appws.NewCreateWorkspace(ws, mem),
		appws.NewListMyWorkspaces(ws),
		appws.NewListMembers(mem),
		appws.NewInviteMember(ws, mem, inv, mailer, cfg.BaseURL),
		appws.NewCreateOrgUnit(mem, units),
		appws.NewListOrgUnits(mem, units),
		appws.NewCreatePosition(mem, positions),
		appws.NewListPositions(mem, positions),
		appws.NewListProducts(products),
		appws.NewEnableProduct(mem, products, wp),
		appws.NewListEnabledProducts(mem, wp),
		apphttp.MustLoadTemplates(),
	)
}
```
Обновить `provideServer` — добавить `hubWS *apphttp.HubWorkspaceHandlers`. Добавить провайдер в `wire.Build`. (Все repo-провайдеры уже есть из W1–W3.)

- [ ] **Step 2: Mount** — `server.go`: расширить `NewRouter` параметром `hubWS *apphttp.HubWorkspaceHandlers`; в существующую hub-группу (где `hub.Register(pr)` под `RequireHubSession(hubStore)`) добавить `hubWS.Register(pr)`. Обновить `server_test.go` вызов `NewRouter` с `apphttp.NewHubWorkspaceHandlers(nil×11, apphttp.MustLoadTemplates())`.

- [ ] **Step 3: Ссылка из аккаунта** — в `templates/account.html` добавить строку `<p><a href="/account/workspaces">Workspaces</a></p>` (рядом с "Log out").

- [ ] **Step 4: Regen + build + test.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && make wire && go build ./... && go vet ./... && go test -short ./...` → чисто/зелёно.

- [ ] **Step 5: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/infrastructure/di/ internal/infrastructure/httpserver/ internal/presentation/http/templates/account.html
git commit -m "feat(hub): wire workspace UI under hub session"
```

---

## Task 4: Финальная проверка + E2E + push

- [ ] **Step 1:** `cd /Users/denisurevic/Downloads/ББД/platform && go test ./... && go vet ./... && go build ./...` → зелёно/чисто.

- [ ] **Step 2: E2E (Docker).** `docker compose up -d --build --wait`; зарегистрировать клиентов (`HYDRA_ADMIN_URL=http://localhost:4445 HUB_CLIENT_SECRET=hub-secret go run ./cmd/bootstrap-client`); создать+verify пользователя (`/signup` + psql UPDATE email_verified). Пройти hub-флоу (cookie-jar) до `/account`, затем:
```bash
# с cookie-jar из авторизованной сессии
curl -sf -b /tmp/hubjar.txt http://localhost:8090/account/workspaces | grep -q 'Create a workspace' && echo "workspaces page OK"
curl -sf -b /tmp/hubjar.txt -c /tmp/hubjar.txt -X POST http://localhost:8090/account/workspaces --data-urlencode 'name=E2E Co' -o /dev/null -w '%{http_code} %{redirect_url}\n'
```
Ожидаем: страница воркспейсов рендерится; POST создаёт (303 на `/account/workspaces/{id}`). (Если браузерный флоу флаки — допустимо доказать через throwaway Go с cookiejar, не коммитить.) Затем `docker compose down`.

- [ ] **Step 3: Push.** `cd /Users/denisurevic/Downloads/ББД/platform && git push origin main`.

---

## Definition of Done (W4)
- Хаб: `GET /account/workspaces` (список + создание), `POST /account/workspaces` (создать → 303).
- Страница воркспейса `GET /account/workspaces/{id}`: участники, подразделения, должности, продукты (включённые + реестр) + формы управления (пригласить/добавить подразделение/должность/включить продукт), все POST → 303 назад.
- Всё под `RequireHubSession`; авторизация мутаций — внутри use-cases (owner/admin). Ссылка на воркспейсы с `/account`.
- Все тесты зелёные; vet/build чисто; E2E-smoke ок; запушено.

## Итог модуля
С завершением W4 Workspace-модуль (W1 ядро + W2 оргструктура + W3 продукты + W4 UI) готов. Аккаунт-хаб получает свитчер воркспейсов и управление. Дальше — интеграция продуктов (Papyrus и др.) как OIDC-клиентов, потребляющих `/me/workspaces` и оргструктуру (P2).
