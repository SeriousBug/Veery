# Veery

Self-hosted web app to manage Docker containers, with passkey-only login. Ships as a single
static Go binary with the web UI embedded (distroless/static base).

## Stack

- **Backend:** Go (module `github.com/SeriousBug/Veery`, `internal/` packages). HTTP via the
  stdlib `net/http` mux with method+path patterns (e.g. `"DELETE /api/users/{id}"`).
- **Frontend:** TypeScript + React 19 SPA in `web/`. TanStack Router + TanStack Query, Ark UI
  components, Panda CSS for styling, `lucide-react` icons, Vite build. Package manager is `pnpm`.
- **Auth:** WebAuthn passkeys only, no passwords. Session cookie. First run prints a one-time
  admin enrollment link to the logs; admins mint single-use invite links from the UI.

## Layout

- `cmd/veery/` — main entrypoint. `veery invite [--normal]` mints a recovery enrollment link from
  the host (full-lockout escape hatch). `veery apply-update --container X --job Y` is what the
  self-update helper container runs; it is not meant to be invoked by hand.
- `internal/api/` — shared request/response types (`types.go`). **Source of truth for TS types.**
- `internal/server/` — HTTP handlers, routing (`server.go`), auth middleware (`middleware.go`,
  `requireAuth`/`requireAdmin`), auth handlers (`auth_handlers.go`).
- `internal/auth/` — WebAuthn, invites, sessions, users.
- `internal/store/` — SQLite persistence (`accessors.go`).
- `internal/docker/`, `internal/metrics/` — container management and host/container metrics.
  Updates are transactional and Veery updates itself via a helper container — see `docs/updates.md`.
- `internal/notify/` — notifications via Shoutrrr service URLs (Discord, ntfy, Slack, webhooks, ...).
  Config comes from `VEERY_NOTIFY_URLS`/`VEERY_NOTIFY_EVENTS` or, unset, from the DB and the UI.
  Targets hold webhook tokens, so the routes are `requireAdmin` and URLs are redacted in logs.
- `web/src/routes/` — page components. `web/src/api/http.ts` — fetch wrapper (`http.get/post/put/del`).
  `web/src/auth/AuthProvider.tsx` — `useAuth()` gives the current `user`.
- `web/embed.go` — embeds `web/dist` into the Go binary via `//go:embed`.

## Type generation

TS API types in `web/src/api/generated.ts` are generated from Go structs in `internal/api` via
**tygo** (`tygo.yaml`). After changing Go API types, regenerate rather than hand-editing the
`.ts` file.

## Build / check

```sh
go build ./...            # backend (needs web/dist to exist for the embed)
cd web && pnpm typecheck  # panda codegen + tsc --noEmit
cd web && pnpm build      # panda codegen + vite build -> web/dist (run before go build)
```

## Conventions

- Admin-only API routes are wrapped in `s.requireAdmin(...)` in `server.go`. Destructive user
  actions (delete user) guard against removing the last admin server-side.
- Frontend destructive actions go through `components/ConfirmDialog.tsx` (`tone="danger"`) and
  report outcomes via the toaster (`lib/toaster.ts`, `toaster.create({ type, title })`).
