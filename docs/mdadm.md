# RAID health (mdadm)

Veery can show the health of Linux software RAID (`mdadm`) arrays on the host: whether every
member disk is up, whether a scrub/resync/recovery is running with its percent and ETA, and a
button (admin-only) to kick off a data scrub. The feature is optional and hides itself completely
when the host has no arrays or the mounts aren't provided.

## How it reads the host

No dependency on the `mdadm` userspace binary (it isn't in the distroless image). Everything comes
from files:

- `${HOST_PROC}/mdstat` (default `/proc/mdstat`): one file listing every array: name, level,
  member devices, the `[n/m] [UU]` up/down field, and the optional progress line during a sync.
- `${HOST_SYS}/block/<md>/md/sync_action` (default `/sys`): the current action even when idle
  (mdstat drops the progress line then), and `mismatch_cnt` for the last check.

`internal/metrics/mdadm.go` (`ScanMdadm`) parses these on the existing metrics poll and attaches
the result to `HostMetrics.Mdadm`, pushed over the WS. `parseMdstat` is a pure function covered by
`mdadm_test.go` fixtures. Starting a scan writes `check` to `sync_action`; the array name is
validated against what mdstat reports first, so the write can't be aimed at an arbitrary path.

## Alerts, last-scan time, and scheduling (`internal/raidwatch`)

`ScanMdadm` is stateless, so a second poller, `raidwatch.Watcher`, adds the state that alerts
and scheduling need. It runs on the metrics poll interval, compares each array against a persisted
baseline (`store` keys `mdadm_notify_baseline`, `mdadm_last_scan`, `mdadm_last_run`,
`mdadm_schedules`), and:

- **Notifies on transitions** through the same notifier as container events. Four events, all
  edge-triggered so a restart doesn't replay them and the first sweep only records:
  - `raid_scan_started` / `raid_scan_finished`: a data-scrub (`check`) begins or ends. Because it
    is a `sync_action` transition, it fires whoever started the scrub: Veery's scheduler, a host
    cron, or a manual `mdadm` command.
  - `raid_unhealthy`: an array crosses into degraded/failed, and again when it recovers.
  - `raid_disk_offline`: a member disk drops out, and again when it rejoins.
- **Records the last-scan time.** The kernel keeps no timestamp for when a scrub last ran, so Veery
  records `time.Now()` when it sees a `check` return to idle and surfaces it as `MdArray.LastScanAt`
  (shown as "Last scan: …" in the UI). `0` means none seen yet.
- **Runs scheduled scrubs.** Per-array schedules are stored as iCal RRULE strings (RFC 5545) and
  evaluated with `github.com/teambition/rrule-go`. On each poll, an array whose schedule has an
  occurrence due since it last ran, and that is currently idle, gets a `check` started. A newly
  saved schedule anchors its "last run" to now, so it never fires for occurrences already past.

### Schedules

Admins edit schedules under Settings (`GET`/`PUT /api/mdadm/schedules`, `requireAdmin`). The UI has
a builder for common cases (e.g. weekly on Sundays at 8PM → `FREQ=WEEKLY;BYDAY=SU;BYHOUR=20;BYMINUTE=0`)
plus a raw RRULE field. Rules are validated with rrule-go before saving.

Schedules are evaluated in the **server's local timezone**. Set the container's `TZ` (e.g.
`TZ=America/New_York`) so "8PM" means 8PM where you are. `GET /api/mdadm/schedules` reports that
zone (`timeZone`, with `timeZoneOffsetSeconds` as a fallback when the name cannot be determined) so
the UI can name it and count down to the next run in the server's terms rather than the browser's.
A scheduled scrub has the same requirements as a manual one, see [Starting scans](#starting-scans).

### When a scan can't start

After writing `check`, Veery reads `sync_action` back and only counts the start as successful if it
reads `check`. A failed start:

- Is stored per array (`store` key `mdadm_scan_failures`) and sent as `MdArray.ScanFailure`, which
  the dashboard shows as a warning under the array with the error, the attempt count, and when the
  next retry is.
- Fires `raid_scan_failed`, so it lands in the event log and any subscribed notification targets.
  A scheduled scrub notifies on its first failure and again when it runs out of retries, not on
  every attempt.
- Is retried, for scheduled scrubs only, after 1, 5, 15 and 60 minutes. After that the scheduler
  gives up on this occurrence and the next occurrence starts a fresh set of attempts. A scrub
  started from the UI is not retried; the admin sees the error right away.

The warning clears once a start succeeds or Veery sees a scrub running on the array, whoever
started it. "Last scan" (`LastScanAt`) is only set when Veery sees a running `check` finish, so a
failed start never updates it.

## Enabling it

Mount the host's `/proc` and `/sys` into the container and point `HOST_PROC` / `HOST_SYS` at them,
the same mounts host CPU/disk metrics already need. That is enough for health and progress, which
only read, so the mounts can be read-only:

```sh
docker run \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -e HOST_PROC=/host/proc \
  -e HOST_SYS=/host/sys \
  ...
```

Health/progress is visible to any logged-in user; starting a scan is admin-only (`requireAdmin`)
and behind a confirm dialog, because a scrub drives host I/O for a long time.

### Starting scans

Starting a scrub, from the UI or a schedule, writes to `/sys/block/<md>/md/sync_action`. That needs
all three of:

1. **`/sys` mounted writable**: drop `:ro` from the `/sys` mount. Otherwise the write fails with
   `read-only file system`.
2. **Running as root**: the md sysfs files are owned by root with mode `0644`, and the image runs as
   uid 65532 by default. Set `--user 0:0` (`user: "0:0"` in compose).
3. **`CAP_SYS_ADMIN`**: the kernel's md sysfs handler (`md_attr_store`) refuses every write without
   it, even from root, and Docker does not grant it by default. Add `--cap-add SYS_ADMIN`
   (`cap_add: [SYS_ADMIN]` in compose).

Without 2 or 3 the write fails with `permission denied`. Docker only grants added capabilities to
root in the container, so `SYS_ADMIN` without `--user 0:0` is not enough.

```sh
docker run \
  --user 0:0 \
  --cap-add SYS_ADMIN \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys \
  -e HOST_PROC=/host/proc \
  -e HOST_SYS=/host/sys \
  ...
```

```yaml
services:
  veery:
    user: "0:0"
    cap_add:
      - SYS_ADMIN
    volumes:
      - /proc:/host/proc:ro
      - /sys:/host/sys
```

`CAP_SYS_ADMIN` and a writable `/sys` give the container broad control over the host kernel. Only
add them if you want Veery to start scrubs; skip them and run scrubs from a host cron instead if you
don't. Veery still notices those scrubs, sends `raid_scan_started`/`raid_scan_finished`, and records
the last-scan time. Running as root also means the `--group-add` for the Docker socket is no
longer needed, though keeping it does no harm.
