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
