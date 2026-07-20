# Event log

Notifications are fire-and-forget: if no channel is configured, or nobody was
looking, the event is gone. The event log records every event the notifier sees
into a table so there is a searchable history of what happened to each service.

## What is recorded

Everything that passes through `Notifier.Notify` (`internal/notify/notify.go`):
container status changes, removals, adoptions, update results, updates
available, and auth events. Recording happens in `Notify`, before the
delivery decision, so **a muted event is still recorded** — muting a channel is
about interruption, not about whether the thing happened. That is what makes it
safe to turn an event (e.g. `container_missing`) off for delivery: the log keeps
the history either way.

Each call may carry an `api.EventMeta{ContainerName, StackID}` so the row links
back to the service it concerns. The docker call sites pass it; auth events name
no service and leave it empty.

## Storage

Table `events` (`internal/store/migrations.go`): `id, event, title, body,
container_name, stack_id, created_at`. No foreign keys tie a row to a container
or stack — a row outlives the service it names, and the columns are plain text
for linking and filtering.

Retention is bounded by the `EventLogRetentionDays` setting (default 30, `0` =
keep forever). Pruning runs on write, in `Notifier.record` → `pruneEvents`:
writes are the only thing that grows the log, so pruning there keeps it bounded
without a background goroutine.

## API

`GET /api/events?cursor=&limit=&event=&container=&stack=&q=` → `api.EventPage`
(`{ items, nextCursor }`). Admin-only: the log includes auth events that name
users.

- **Cursor pagination** on `(created_at, id)` descending. The list grows at the
  head while a user reads it; offsets would skip and duplicate rows, a cursor
  does not. `nextCursor` is empty once the oldest event has been returned.
- **Filter** by event type (`event`) and by service (`container`, `stack`).
- **Search** across title and body (`q`) via `LIKE` (wildcards escaped). At a
  home server's row counts this is fine; it can move to FTS5 behind the same
  endpoint later.

## Live updates

Each recorded event is broadcast as a `WSTypeEvent` message
(`internal/server/ws.go`). The hub only sends event frames to **admin** WS
clients, since non-admins have no event page and the log names users. The
Events page prepends live items above the first fetched page, so a cursor page
already loaded stays valid and what the user is reading does not shift.

## UI

`web/src/routes/Events.tsx`: reverse-chronological list grouped by day, a filter
by event type, a search box, and cursor-driven "Load more" via
`useInfiniteQuery`. Each row links to the service it concerns (its `stackId`, or
the stack whose containers include `containerName`).

## Not in scope

An **audit trail** — lifecycle actions attributed to the user who took them
(started/stopped/updated by whom) — is a stronger, separate thing. It needs the
user id plumbed from the request into the docker jobs and is left as a
follow-up.
