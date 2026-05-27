# OurNeZt Web

Gin-based server-rendered web app for Phase 1 of OurNeZt. It connects to `OurNeZt-core` over gRPC and provides browser routes for auth, admin user creation, family management, people profiles, housing options, and dashboard views.

## Stack

- Go
- Gin
- gRPC client stubs
- HTML templates
- Tailwind (CDN) + DaisyUI (CDN)

## Quick Start

1. Start `OurNeZt-core` and PostgreSQL first.
2. In this folder, set environment variables:

```bash
export APP_ENV=development
export WEB_ADDR=:8080
export CORE_GRPC_ADDR=localhost:50051
export SESSION_COOKIE_NAME=ournezt_session
export SESSION_COOKIE_MAX_AGE=24h
export SESSION_COOKIE_SECURE=false
export REQUEST_TIMEOUT=5s
```

3. Run the web app:

```bash
go run ./cmd/web
```

Then open `http://localhost:8080`.

## Routes

Implemented Phase 1 page routes:

- `/login`, `/logout`
- `/bootstrap-admin-help`
- `/change-password`
- `/admin`
- `/admin/users`
- `/families`, `/families/:id`
- `/dashboard`
- `/people`
- `/housing`, `/housing/compare`
- `/settings`

## Notes

- Session token is stored in an HTTP-only cookie and sent to core as gRPC metadata (`x-session-token`).
- First admin provisioning is handled by `ournezt-core` startup bootstrap env vars (`BOOTSTRAP_ADMIN_EMAIL`, `BOOTSTRAP_ADMIN_PASSWORD`, optional `BOOTSTRAP_ADMIN_DISPLAY_NAME`).
- If `must_change_password=true`, the user is forced through `/change-password` before other authenticated routes.
- Admin actions in the web UI rely on authenticated admin sessions.
- Protobuf stubs are currently copied from `OurNeZt-core` for local development. In the target architecture, these should come from a shared `ournezt-proto` repository.
