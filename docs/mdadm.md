# RAID health (mdadm)

Veery can show the health of Linux software RAID (`mdadm`) arrays on the host: whether every
member disk is up, whether a scrub/resync/recovery is running with its percent and ETA, and a
button (admin-only) to kick off a data scrub. The feature is optional and hides itself completely
when the host has no arrays or the mounts aren't provided.

## How it reads the host

No dependency on the `mdadm` userspace binary (it isn't in the distroless image). Everything comes
from files:

- `${HOST_PROC}/mdstat` (default `/proc/mdstat`) — one file listing every array: name, level,
  member devices, the `[n/m] [UU]` up/down field, and the optional progress line during a sync.
- `${HOST_SYS}/block/<md>/md/sync_action` (default `/sys`) — the current action even when idle
  (mdstat drops the progress line then), and `mismatch_cnt` for the last check.

`internal/metrics/mdadm.go` (`ScanMdadm`) parses these on the existing metrics poll and attaches
the result to `HostMetrics.Mdadm`, pushed over the WS. `parseMdstat` is a pure function covered by
`mdadm_test.go` fixtures. Starting a scan writes `check` to `sync_action`; the array name is
validated against what mdstat reports first, so the write can't be aimed at an arbitrary path.

## Alerts, last-scan time, and scheduling (`internal/raidwatch`)

`ScanMdadm` is stateless, so a second poller — `raidwatch.Watcher` — adds the state that alerts
and scheduling need. It runs on the metrics poll interval, compares each array against a persisted
baseline (`store` keys `mdadm_notify_baseline`, `mdadm_last_scan`, `mdadm_last_run`,
`mdadm_schedules`), and:

- **Notifies on transitions** through the same notifier as container events. Four events, all
  edge-triggered so a restart doesn't replay them and the first sweep only records:
  - `raid_scan_started` / `raid_scan_finished` — a data-scrub (`check`) begins or ends. Because it
    is a `sync_action` transition, it fires whoever started the scrub: Veery's scheduler, a host
    cron, or a manual `mdadm` command.
  - `raid_unhealthy` — an array crosses into degraded/failed, and again when it recovers.
  - `raid_disk_offline` — a member disk drops out, and again when it rejoins.
- **Records the last-scan time.** The kernel keeps no timestamp for when a scrub last ran, so Veery
  records `time.Now()` when it sees a `check` return to idle and surfaces it as `MdArray.LastScanAt`
  (shown as "Last scan: …" in the UI). `0` means none seen yet.
- **Runs scheduled scrubs.** Per-array schedules are stored as iCal RRULE strings (RFC 5545) and
  evaluated with `github.com/teambition/rrule-go`. On each poll, an array whose schedule has an
  occurrence due since it last ran — and that is currently idle — gets a `check` started. A newly
  saved schedule anchors its "last run" to now, so it never fires for occurrences already past.

### Schedules

Admins edit schedules under Settings (`GET`/`PUT /api/mdadm/schedules`, `requireAdmin`). The UI has
a builder for common cases (e.g. weekly on Sundays at 8PM → `FREQ=WEEKLY;BYDAY=SU;BYHOUR=20;BYMINUTE=0`)
plus a raw RRULE field. Rules are validated with rrule-go before saving.

Schedules are evaluated in the **server's local timezone** — set the container's `TZ` (e.g.
`TZ=America/New_York`) so "8PM" means 8PM where you are. A scheduled scrub, like a manual one, needs
`/sys` mounted **writable**; without it the scan-start write fails and is logged.

## Enabling it

Mount the host's `/proc` and `/sys` into the container and point `HOST_PROC` / `HOST_SYS` at them —
the same mounts host CPU/disk metrics already need. Starting a scan additionally needs `/sys`
mounted **writable** (health/progress display is read-only):

```sh
docker run \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys \
  -e HOST_PROC=/host/proc \
  -e HOST_SYS=/host/sys \
  ...
```

Health/progress is visible to any logged-in user; starting a scan is admin-only (`requireAdmin`)
and behind a confirm dialog, because a scrub drives host I/O for a long time.
