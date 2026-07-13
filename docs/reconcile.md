# Reconcile

Veery does not create or edit containers. The user's compose file (or their `docker run`) is the
source of truth, and they are free to change it at any time, including while Veery is not running.
`internal/docker/reconcile.go` is what keeps Veery's records honest when they do.

## What Veery holds, and how it goes stale

Each managed container has a row with a **snapshot**: the full create-spec captured from
`docker inspect`. The snapshot is what `Update` and `BringUpStack` recreate the container *from*, so
a snapshot that no longer matches reality is not a cosmetic problem — it is a spec that will
overwrite the user's own.

Three things happen out of band:

| The user does | Veery sees |
| --- | --- |
| adds a service to a stack | a container with no managed row |
| removes a service from a stack | a managed row with no container |
| edits a service (`compose up -d`) | a container that is not the one it snapshotted |

## Detecting an edit: the container id

`managed_containers.container_id` records the container the snapshot was taken from. Docker never
reuses a container id, and starting, stopping or restarting keeps it — only a *recreate* mints a new
one. So `live.ID != container_id` means something other than Veery built this container, and its
spec is one Veery has never seen. There is no config diffing: nothing to tune, and no false
positives from runtime fields.

Every path that creates a container records the id it created (`Adopt`, the update swap,
`BringUpStack`), so the only way for the ids to diverge is an outside recreate.

Rows written before the column existed carry an empty id and are backfilled on the first sweep.

## The settled gate

A re-snapshot only happens once the container has **settled**: running and passing its health check,
or deliberately not running (created, paused, or exited cleanly). A container that is crash-looping,
dead, or exited non-zero has proved nothing, and its spec is not one to keep — bring-up and update
rollback build from the snapshot, so recording a spec that cannot run is how a service stays down.
Until it settles, the previous known-good snapshot stands.

## Where it runs

- At startup, after `Recover`, which picks up everything that changed while Veery was down.
- On the status sweep (`pollStacks`), for changes made while it is up.
- Inside `doUpdate` and `ApplyUpdate`, under the container lock, before the pull. This is the one
  that matters: it makes the update path safe on its own, no matter how the sweep is timed. Without
  it, updating a container whose tag the user changed pulls the image they moved *away* from and
  presents the downgrade as an update.

The sweep takes the container lock with `TryLock` and skips anything held: an update owns the
containers it is swapping and records what it creates itself.

## Removal is not reconciled

A managed row whose container is gone is reported as missing, never deleted. Docker keeps no
tombstone, so `compose down` (bring it back) and `up -d --remove-orphans` (it is gone for good) are
indistinguishable, and guessing wrong means discarding the snapshot bring-up needs. The user says
which: **Bring back up**, or **Forget it** (`DELETE /api/containers/{id}/managed`).

## Notifications

Reconcile touches things without being asked, so it says so, on events the user can turn off
independently (`internal/api/types.go`, toggled in Settings):

- `container_adopted` — Veery started managing a container that appeared in a managed stack.
- `container_missing` — a managed container was removed. Split out of `container_status` because on
  a host whose compose files change often it is the noisiest event and usually reports the user's
  own work back to them. A stack that goes *whole* is one message ("blog was taken down"), not one
  per container.
- `container_status` — crashes, unhealthy, stopped, recovered. What is actually going wrong.

## Removal, continued

What Veery *can* tell apart is a stopped container from a removed one — a stopped container still
exists, so a reboot or a crash-loop never reads as missing. And a stack whose containers are *all*
missing was taken down whole, which is a thing users do on purpose, so it reports as missing rather
than needs-attention. A container missing from a service whose other parts are still running has no
such explanation, and that one is flagged.
