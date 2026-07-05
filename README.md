# Papyrus Identity Center

Централизованный сервис идентичности (OAuth2/OIDC) для экосистемы Papyrus:
единый аккаунт и SSO, к которому подключаются продукты (Papyrus, Lite и др.).

## Архитектура

Гибрид: **Ory Hydra** (сертифицированный OAuth2/OIDC-движок — токены, крипто) +
наш сервис на **Go** (login/consent UI, хранилище пользователей, профиль, сессии).
Чистая архитектура, DI (google/wire), TDD.

- **Identity** — аккаунты, регистрация, верификация email, сброс пароля, логин, сессии.
- **Sessions** — богатая инфа (устройство/IP), завершение по устройству и «выйти везде».
- Consent для доверенных (first-party) клиентов — авто-принятие.

Подробности — в `docs/specs/` (дизайн) и `docs/plans/` (планы реализации по фазам).

## Стек

Go 1.26 · Ory Hydra v2.2 · PostgreSQL · Redis · chi · pgx · golang-migrate ·
google/wire · testify · testcontainers · Docker Compose.

## Запуск (Docker)

```bash
cp .env.example .env
docker compose up -d --build --wait
curl -sf http://localhost:8090/healthz   # {"status":"ok"}
```

Сервисы: platform-core (:8090), Hydra (public :4444, admin :4445),
Postgres (:5440), Redis (:6390).

## Разработка

```bash
make test-unit    # юнит-тесты (без Docker)
make test         # + интеграционные (testcontainers, нужен Docker)
make wire         # регенерация DI
make run          # локальный запуск
```

## Статус

Реализовано: каркас, identity core (рега/верификация/сброс), Hydra login/consent +
сессии, завершение сессий + auth API. Дальше: аккаунт-хаб UI, воркспейсы/оргструктура.
