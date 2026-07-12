# Veery

A tiny, self-hosted web app to manage your Docker containers — with **passkey-only login** (no
passwords), one-tap service control, updates (manual and automatic), and live host + container
resource metrics. Built for the person who knows "Home Assistant is down, I'll restart it," not
Docker internals.

Veery ships as a single static Go binary with the web UI embedded, on a `distroless/static` base,
so the image stays small.

## What it does

- **Passkey-only auth.** No passwords to phish or brute-force. First run prints a one-time admin
  enrollment link to the logs; admins mint further single-use invite links.
- **Adopt then manage.** Veery snapshots each container's full create-spec from `docker inspect`
  and stores it. From then on it can stop / start / restart / update a service, and **recreate it
  from the snapshot** if the container is removed or the host reboots.
- **Updates.** Manual (pull → compare image digest → recreate if changed) and an auto-update poller.
- **Live metrics.** Host CPU, memory, disk usage and disk read/write bandwidth, plus per-container
  CPU and memory, pushed live over a WebSocket into colorful gauges.
- **Proactive.** Anything unhealthy, stopped, or restart-looping surfaces in a "Needs attention"
  band with a one-tap fix.

### Scope

Veery **adopts and manages** existing stacks. It does not include a compose engine, so it cannot
create a stack that has never run. Bring your stack up your usual way once (or point Veery at one
that is already running); Veery snapshots it, and from then on can control, update, and rebuild it
from the snapshot. The "Bring back up" button recreates a stack from its stored snapshot.

## Requirements

- Docker Engine with the API socket available at `/var/run/docker.sock`.
- **A TLS reverse proxy.** WebAuthn (passkeys) requires a secure context, and Veery's session
  cookie is `Secure`. Run Veery behind a proxy that terminates HTTPS (Caddy, Traefik, nginx, …).
  `localhost` is also a secure context, which is enough for local development.

## Configuration

| Env var         | Default                  | Meaning                                             |
| --------------- | ------------------------ | --------------------------------------------------- |
| `VEERY_RP_ID`   | `localhost`              | WebAuthn Relying Party ID — your domain, e.g. `veery.example.com` (no scheme, no port). |
| `VEERY_ORIGIN`  | `http://localhost:8080`  | Full origin the browser uses, e.g. `https://veery.example.com`. Used for invite URLs and cookie `Secure` flag. |
| `VEERY_ADDR`    | `:8080`                  | Listen address.                                     |
| `VEERY_DB`      | `/data/veery.db`         | SQLite database path (persist this volume).         |
| `HOST_PROC`     | (unset)                  | Set to `/host/proc` when running in a container so host metrics are read from the mounted host `/proc`. |
| `HOST_SYS`      | (unset)                  | Set to `/host/sys` likewise for host `/sys`.        |

## Running

```sh
docker run -d --name veery \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -v veery-data:/data \
  -e VEERY_RP_ID=veery.example.com \
  -e VEERY_ORIGIN=https://veery.example.com \
  -e HOST_PROC=/host/proc \
  -e HOST_SYS=/host/sys \
  ghcr.io/seriousbug/veery:latest
```

Then read the first-run enrollment link from the logs and open it to register your admin passkey:

```sh
docker logs veery
# veery no users yet — enroll the first admin passkey here:
#     https://veery.example.com/enroll?token=...
```

Put Veery behind your TLS proxy so the browser reaches `VEERY_ORIGIN` over HTTPS.

### Recovering access (lost all passkeys)

Passkeys have no password fallback, so if every enrolled device is lost you would
otherwise be locked out. Because host access already equals full control (the Docker
socket), you can mint a fresh enrollment link from the host and enroll a new passkey:

```sh
docker exec veery /veery invite            # admin link (default)
docker exec veery /veery invite --normal   # non-admin link
```

The link is single-use and valid for 24 hours. Open it to register a new passkey.

## Security notes

- **Docker socket access is root-equivalent on the host.** Anyone who can authenticate to Veery can
  control your containers. Passkey-only auth is the gate — there is no password fallback.
- Veery must run behind TLS: WebAuthn needs a secure context and the session cookie is `Secure`.
- Invites are single-use and expiring; sessions use random, expiring tokens in an `HttpOnly` cookie
  that JavaScript never sees. The same cookie authenticates the WebSocket upgrade.
- Mount `/proc` and `/sys` read-only. The Docker socket must allow API calls (it cannot be read-only
  for write operations like start/stop), so treat access to Veery as access to the host.

## Development

Tooling versions are pinned in `.tool-versions` (Go, Node, pnpm). The frontend uses pnpm; a 7-day
minimum-release-age is enforced (pnpm `minimumReleaseAge`; Go via pinned module versions).

```sh
# Backend (serves the last-built embedded SPA, or run the Vite dev server alongside)
go run ./cmd/veery

# Frontend dev server (proxies /api, /auth, /ws to :8080)
cd web && pnpm install && pnpm dev

# Regenerate shared TypeScript types from the Go structs (single source of truth)
go generate ./internal/api/

# Build the tiny production image
docker build -t veery .
```

Shared types live in `internal/api/types.go` and are generated into `web/src/api/generated.ts` with
[tygo](https://github.com/gzuidhof/tygo), so the backend and frontend stay typed off one source.
