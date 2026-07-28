# Updates

## Checking for updates

`internal/docker/updatecheck.go` inspects each managed container's remote image digest without
pulling and records an in-memory `updateAvailable` flag (`CheckUpdates`). `UpdateCheckPoller` runs
this on the auto-update interval; the flags ride the WS stacks push.

Users can also force a check instead of waiting for the poller. `checkUpdates` is the shared scoped
sweep; `CheckContainerUpdates`/`CheckStackUpdates`/`CheckAllUpdates` scope it to one container, one
stack, or everything, behind `POST /api/containers/{id}/check-update`, `POST /api/stacks/{id}/check-update`,
and `POST /api/updates/check`. These run synchronously so the UI can await the result. The "Up to
date" pill (`ActionBar`) and the dashboard "Check for updates" button trigger the matching scope. A
newly found update still flips the UI via the same WS push, so a manual check and the poller converge.

## Transactional swap

`internal/docker/update.go` pulls the image, and if the digest actually changed, swaps the container
onto it. The old container is *parked* (renamed to `<name>__veery_old` and stopped, but kept) while
the new one is created and verified:

- New container comes up healthy within `verifyTimeout` → the parked container is removed.
- It does not → the new one is removed, and the parked container is renamed back and started.

So a parked container only exists while a swap is in flight. Finding one at startup means a swap was
interrupted, which is what `Recover` keys off.

A bad image therefore cannot leave a service down. The confirm dialog in the UI promises this, so it
is a guarantee, not a best effort.

## Giving up on an update

An update that fails is rolled back, so the service survives it — but the poller comes back on the
next interval and tries the same broken image again, taking the service down and bringing it back
every hour, forever. `internal/docker/autoupdate.go` bounds that, in two steps:

- **Per version.** After `maxVersionAttempts` (3) failed auto-updates onto the same version, that
  version is written off: the poller skips it until the registry serves a different one. The version
  is identified by the digest the registry reports for the container's image tag
  (`targetVersion`, the same `DistributionInspect` the update check uses), so a newly published
  release starts on a clean count. One bad release is the normal case and the next one usually
  fixes it.
- **Per container.** When `maxFailedVersions` (3) versions in a row have been written off, the
  problem is not the release: the container cannot come up on a new image at all. Auto-update is
  switched off for it and it keeps running the version it is on, until the user turns it back on.

Both are announced under the `auto_update_stopped` event, its own alert category because it is the
one update event that needs the user to do something: a service that has quietly stopped updating is
exactly what auto-update was turned on to prevent.

Counts live in `update_failures` (`internal/store/update_failures.go`), keyed by container name and
target digest. They are dropped when an update succeeds, when the user turns auto-update back on
(that is the user saying to start over), and when the container stops being managed.

Every update carries an `api.Source` saying who asked for it — `SourceUser` or `SourceAutomation` —
passed in by the caller: `Manager.Update(ctx, id, src)` from the HTTP handler, `SourceAutomation`
from the poller. Two attempts are deliberately **not** counted:

- One a **user** asked for. Someone is watching the outcome and can decide for themselves whether to
  try again; a person retrying a broken image must not be what turns their auto-updates off.
- One where the registry could not be reached, so `targetVersion` is empty. There is no version to
  blame, and a network outage must not read as three bad releases.

A self-update is counted like any other, even though the process that finishes it is not the one
that started it: the `source` and `target` columns on `update_jobs` carry both across the handoff for
`ApplyUpdate` to read back.

### Off by choice vs. off because Veery gave up

A toggle that is off looks the same either way and means opposite things: one is a settled choice,
the other is a service that is stuck and stays behind until somebody looks at it. The same
`api.Source` tells them apart: `SetAutoUpdate(id, on, src)` records who last set it in
`managed_containers.auto_update_source`, so off + `automation` is Veery having given up and off +
`user` is a choice. The HTTP toggle always passes `SourceUser`, in either direction, which is what
makes turning it back on hand the container back to the user.

It rides out on `api.Container.AutoUpdateSource`. `AutoUpdateToggle` turns the card red and explains
what happened and where to look, rather than presenting an off switch with no explanation, and the
"update available" notification says which of the two it is for the same reason.

## Veery updating itself

Veery cannot swap its own container in-process. Parking it means stopping it, and stopping it kills
the process that would go on to create and verify the replacement: the container ends up parked, the
replacement never gets created, and Veery is down until someone renames it back by hand.

So `handOff` (`selfupdate.go`) starts a detached helper container that runs `veery apply-update` and
performs the swap from outside. Points that are load-bearing:

- The helper runs the image Veery is running **now**, not the image being updated to. Its job is to
  verify the new version and roll back if it is bad; running it *on* the new image would mean a broken
  image takes down the thing meant to recover from it.
- Its entrypoint is copied from Veery's own container rather than assumed from the image.
- It gets no published ports (the old Veery still holds them) and no restart policy.
- It is labelled `veery.role=updater`, which keeps it out of the stack list and gets it pruned on the
  next start.

## Recovery

`Manager.Recover` (`recover.go`) runs at startup, before serving, and reconciles anything left
half-done: a crash, a host reboot, or the handoff above.

It is driven off Docker state, not the DB, because Docker state is what survives a hard kill: a
container parked under the `__veery_old` name means a swap was in progress, whatever the DB says. For
each parked container it either restores it (no replacement, or the replacement is unhealthy) or
retires it (the replacement is healthy, or *is* the process now running).

It deliberately does nothing while a helper container is still running: that helper still needs the
parked container as its rollback target, and reconciling underneath it would tear that target out.

## Jobs

Update jobs are persisted (`store/update_jobs.go`) because the process that starts a self-update is
never the process that finishes it. A client that connects gets the whole job picture (`WSTypeJobs`):
the updates in flight, plus the ones that finished recently. Without the latter, an update that
completes while the browser is disconnected, which is *every* self-update, leaves the UI spinning on
a job it never sees resolve.

## Testing

The unit and integration tests cover the swap, the handoff spec, and each recovery decision against a
real daemon. They all stub out either the container or the binary.

`TestE2ESelfUpdate` (`selfupdate_e2e_test.go`) is the one that proves the real thing: it builds the
actual distroless image, pushes it to a throwaway registry, runs Veery in a container, and has it
replace itself. It is slow and skipped by default:

```sh
VEERY_E2E=1 go test ./internal/docker/ -run TestE2ESelfUpdate -timeout 25m -v
```
