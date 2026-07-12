# Veery

A self-hosted web app to manage your Docker containers, with passkey-only login (no passwords),
service start/stop/restart, updates (manual and automatic), and live host and container resource
metrics. It is meant for people who want to restart Home Assistant without learning Docker.

Veery ships as a single static Go binary with the web UI embedded, on a `distroless/static` base,
so the image stays small.

## What it does

- Passkey-only auth. There are no passwords to phish or brute-force. The first run prints a
  one-time admin enrollment link to the logs; admins can then mint further single-use invite links.
- Adopt, then manage. Veery snapshots each container's full create-spec from `docker inspect` and
  stores it. From then on it can stop, start, restart, or update a service, and recreate it from the
  snapshot if the container is removed or the host reboots.
- Updates. Manual (pull the image, compare its digest, recreate the container if it changed) and an
  auto-update poller.
- Live metrics. Host CPU, memory, disk usage and disk read/write bandwidth, plus per-container CPU
  and memory, pushed over a WebSocket into gauges.
- Anything unhealthy, stopped, or restart-looping shows up in a "Needs attention" band with a
  button to fix it.
- Notifications. Veery can tell you when a service breaks or comes back, when an update lands or
  fails, when a new image is out, and when someone signs in. It sends to Discord, ntfy, Slack,
  Telegram, Gotify, Pushover, Matrix, email, or a plain webhook.

### Scope

Veery adopts and manages existing stacks. It has no compose engine, so it cannot create a stack that
has never run. Bring your stack up your usual way once, or point Veery at one that is already
running. Veery snapshots it, and from then on can control, update, and rebuild it from that
snapshot. The "Bring back up" button recreates a stack from its stored snapshot.

## Requirements

- Docker Engine with the API socket available at `/var/run/docker.sock`.
- A TLS reverse proxy. WebAuthn (passkeys) requires a secure context, and Veery's session cookie is
  `Secure`. Run Veery behind a proxy that terminates HTTPS, such as Caddy, Traefik, or nginx.
  `localhost` is also a secure context, which is enough for local development.

## Configuration

| Env var        | Default                 | Meaning                                                                                                       |
| -------------- | ----------------------- | ------------------------------------------------------------------------------------------------------------- |
| `VEERY_RP_ID`  | `localhost`             | WebAuthn Relying Party ID: your domain, e.g. `veery.example.com` (no scheme, no port).                         |
| `VEERY_ORIGIN` | `http://localhost:8080` | Full origin the browser uses, e.g. `https://veery.example.com`. Used for invite URLs and the cookie `Secure` flag. |
| `VEERY_ADDR`   | `:8080`                 | Listen address.                                                                                                |
| `VEERY_DB`     | `/data/veery.db`        | SQLite database path (persist this volume).                                                                    |
| `HOST_PROC`    | (unset)                 | Set to `/host/proc` when running in a container, so host metrics are read from the mounted host `/proc`.        |
| `HOST_SYS`     | (unset)                 | Set to `/host/sys` likewise for host `/sys`.                                                                   |
| `VEERY_NOTIFY_URLS` | (unset)            | Where to send notifications, as whitespace-separated [Shoutrrr](https://containrrr.dev/shoutrrr/v0.8/services/overview/) URLs. Setting this makes notifications read-only in the UI. |
| `VEERY_NOTIFY_EVENTS` | (unset)          | Which events to send, comma-separated: `container_status`, `update_applied`, `update_available`, `auth`. Unset means all of them. Only read when `VEERY_NOTIFY_URLS` is set. |

### Notifications

Configure them under Settings (admins only), or pin them with the environment variables above. A
target is a Shoutrrr service URL, so most notification services work:

```sh
VEERY_NOTIFY_URLS="discord://token@channel-id ntfy://ntfy.sh/my-topic"
VEERY_NOTIFY_EVENTS="container_status,update_applied"
```

To get the Discord form, take the webhook URL Discord gives you
(`https://discord.com/api/webhooks/<channel-id>/<token>`) and write it as `discord://<token>@<channel-id>`.

Veery sends four kinds of event, each of which can be switched off:

| Event | Sent when |
| ----- | --------- |
| `container_status` | A container Veery manages stops, crashes, goes unhealthy, disappears, or comes back up. Containers you have not adopted are ignored. |
| `update_applied` | An update finished, or failed and was rolled back. |
| `update_available` | A newer image is out for a managed container that does not auto-update. |
| `auth` | Someone signs in, or a passkey is enrolled. |

Notification URLs contain webhook tokens and passwords, so only admins can read or change them, and
they are never written to the logs.

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
# veery no users yet - enroll the first admin passkey here:
#     https://veery.example.com/enroll?token=...
```

Put Veery behind your TLS proxy so the browser reaches `VEERY_ORIGIN` over HTTPS.

Images are published to `ghcr.io/seriousbug/veery`. Use a version tag like `v0.1.0` to pin a
release, `latest` for the newest release, or `dev` for the current state of `main`.

### Recovering access (lost all passkeys)

Passkeys have no password fallback, so if every enrolled device is lost you would otherwise be
locked out. Host access already means full control through the Docker socket, so you can mint a
fresh enrollment link from the host and enroll a new passkey:

```sh
docker exec veery /veery invite            # admin link (default)
docker exec veery /veery invite --normal   # non-admin link
```

The link is single-use and valid for 24 hours. Open it to register a new passkey.

## Security notes

- Docker socket access is root-equivalent on the host. Anyone who can authenticate to Veery can
  control your containers. Passkey-only auth is the gate, and there is no password fallback.
- Veery must run behind TLS: WebAuthn needs a secure context and the session cookie is `Secure`.
- Invites are single-use and expiring. Sessions use random, expiring tokens in an `HttpOnly` cookie
  that JavaScript never sees. The same cookie authenticates the WebSocket upgrade.
- Mount `/proc` and `/sys` read-only. The Docker socket must allow API calls, since it cannot be
  read-only for operations like start and stop, so treat access to Veery as access to the host.

## Limitations and roadmap

- Private registries. Image updates pull anonymously, so private images (private GHCR or Docker Hub
  repos, self-hosted registries that need auth) can't be updated yet. Per-registry credentials in
  settings are planned.
- No log viewer yet. To find out why a service is down you still need `docker logs <name>` on the
  host. An in-app log tail is planned.
- Snapshot drift. Veery snapshots a container's spec when you first manage it. If you later change
  that stack your usual way (new port, env, volume) and bring it up outside Veery, Veery's snapshot
  is stale. Re-run "Let Veery manage this" on it to refresh the snapshot before using "Bring back
  up".
- Startup ordering. Bringing a whole stack back up does not yet honor `depends_on` ordering, so
  services that depend on each other may need a moment and a retry to settle.

## Development

Tooling versions are pinned in `.tool-versions` (Go, Node, pnpm). The frontend uses pnpm, with a
7-day minimum release age enforced (pnpm `minimumReleaseAge`; Go through pinned module versions).

```sh
# Backend (serves the last-built embedded SPA, or run the Vite dev server alongside)
go run ./cmd/veery

# Frontend dev server (proxies /api, /auth, /ws to :8080)
cd web && pnpm install && pnpm dev

# Regenerate shared TypeScript types from the Go structs (single source of truth).
# Run from the repo root: tygo reads tygo.yaml from the working directory.
go run github.com/gzuidhof/tygo@v0.2.17 generate

# Build the production image
docker build -t veery .
```

Shared types live in `internal/api/types.go` and are generated into `web/src/api/generated.ts` with
[tygo](https://github.com/gzuidhof/tygo), so the backend and frontend stay typed off one source.

## License

Veery is licensed under the GNU Affero General Public License v3.0 only (AGPL-3.0-only). See
[LICENSE](LICENSE).
