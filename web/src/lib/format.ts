const UNITS = ["B", "KB", "MB", "GB", "TB", "PB"] as const;

/** Human-friendly bytes, e.g. 3.2 GB. */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    UNITS.length - 1,
  );
  const val = bytes / 1024 ** i;
  const rounded = val >= 100 || i === 0 ? Math.round(val) : val.toFixed(1);
  return `${rounded} ${UNITS[i]}`;
}

/** Bandwidth, e.g. 12 MB/s. */
export function formatRate(bytesPerSec: number): string {
  return `${formatBytes(bytesPerSec)}/s`;
}

/** Whole-number percent, clamped to 0–100 for display. */
export function formatPercent(pct: number): string {
  return `${Math.round(clampPct(pct))}%`;
}

export function clampPct(pct: number): number {
  if (!Number.isFinite(pct)) return 0;
  return Math.max(0, Math.min(100, pct));
}

/** "3.2 GB / 8 GB" style pair. */
export function formatUsage(used: number, total: number): string {
  return `${formatBytes(used)} / ${formatBytes(total)}`;
}

export function ratioPct(used: number, total: number): number {
  if (!total || total <= 0) return 0;
  return clampPct((used / total) * 100);
}

/** Friendly relative age from a unix-seconds timestamp. */
export function formatAge(unixSeconds: number): string {
  if (!unixSeconds) return "unknown";
  const secs = Math.max(0, Math.floor(Date.now() / 1000 - unixSeconds));
  if (secs < 60) return "just now";
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins} min${mins === 1 ? "" : "s"} ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours} hour${hours === 1 ? "" : "s"} ago`;
  const days = Math.floor(hours / 24);
  return `${days} day${days === 1 ? "" : "s"} ago`;
}
