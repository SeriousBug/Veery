# Updates

## Transactional swap

`internal/docker/update.go` pulls the image, and if the digest actually changed, swaps the container
onto it. The old container is *parked* — renamed to `<name>__veery_old` and stopped, but kept — while
the new one is created and verified:

- New container comes up healthy within `verifyTimeout` → the parked container is removed.
- It does not → the new one is removed, and the parked container is renamed back and started.

So a parked container only exists while a swap is in flight. Finding one at startup means a swap was
interrupted, which is what `Recover` keys off.

A bad image therefore cannot leave a service down. The confirm dialog in the UI promises this, so it
is a guarantee, not a best effort.

## Veery updating itself

Veery cannot swap its own container in-process. Parking it means stopping it, and stopping it kills
the process that would go on to create and verify the replacement — the container ends up parked, the
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
half-done — a crash, a host reboot, or the handoff above.

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
completes while the browser is disconnected — which is *every* self-update — leaves the UI spinning on
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
