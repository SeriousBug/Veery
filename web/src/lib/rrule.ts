import { RRule } from "rrule";

export type Freq = "daily" | "weekly" | "monthly";

// BYDAY tokens in week order, Sunday first, matching the button row.
export const WEEKDAYS: { token: string; short: string; label: string }[] = [
  { token: "SU", short: "S", label: "Sunday" },
  { token: "MO", short: "M", label: "Monday" },
  { token: "TU", short: "T", label: "Tuesday" },
  { token: "WE", short: "W", label: "Wednesday" },
  { token: "TH", short: "T", label: "Thursday" },
  { token: "FR", short: "F", label: "Friday" },
  { token: "SA", short: "S", label: "Saturday" },
];

export interface Builder {
  freq: Freq;
  weekdays: string[]; // BYDAY tokens, e.g. ["SU"]
  monthday: number; // 1-31
  hour: number; // 0-23
  minute: number; // 0-59
}

export const defaultBuilder: Builder = {
  freq: "weekly",
  weekdays: ["SU"],
  monthday: 1,
  hour: 20,
  minute: 0,
};

// buildRRule turns builder fields into a bare RRULE string (no "RRULE:" prefix),
// the form Veery stores and rrule-go parses on the backend.
export function buildRRule(b: Builder): string {
  const parts = [`FREQ=${b.freq.toUpperCase()}`];
  if (b.freq === "weekly" && b.weekdays.length > 0) {
    parts.push(`BYDAY=${b.weekdays.join(",")}`);
  }
  if (b.freq === "monthly") {
    parts.push(`BYMONTHDAY=${b.monthday}`);
  }
  parts.push(`BYHOUR=${b.hour}`, `BYMINUTE=${b.minute}`);
  return parts.join(";");
}

function stripPrefix(rule: string): string {
  return rule.replace(/^RRULE:/i, "").trim();
}

// describeRRule returns a human sentence for a rule, including the time of day,
// which rrule.js's own toText() omits. Returns null when the rule is unparseable.
export function describeRRule(rule: string): string | null {
  const bare = stripPrefix(rule);
  if (!bare) return null;
  let text: string;
  try {
    text = RRule.fromString(bare).toText();
  } catch {
    return null;
  }
  const time = timeFromRRule(bare);
  return time ? `${text} at ${time}` : text;
}

function pad(n: number): string {
  return n.toString().padStart(2, "0");
}

// formatTime renders 24h hour/minute as a 12h clock label, e.g. "8:00 PM".
export function formatTime(hour: number, minute: number): string {
  const ampm = hour < 12 ? "AM" : "PM";
  const h = hour % 12 === 0 ? 12 : hour % 12;
  return `${h}:${pad(minute)} ${ampm}`;
}

function timeFromRRule(bare: string): string | null {
  const h = matchInt(bare, "BYHOUR");
  if (h === null) return null;
  const m = matchInt(bare, "BYMINUTE") ?? 0;
  return formatTime(h, m);
}

function matchInt(bare: string, key: string): number | null {
  for (const attr of bare.split(";")) {
    const [k, v] = attr.split("=");
    if (k.toUpperCase() === key && v) {
      const n = parseInt(v.split(",")[0], 10);
      if (!Number.isNaN(n)) return n;
    }
  }
  return null;
}

// parseBuilder recovers builder fields from a stored rule so the UI can prefill
// its controls. Returns null for rules the simple builder can't represent (the
// raw editor still holds the exact rule).
export function parseBuilder(rule: string): Builder | null {
  const bare = stripPrefix(rule);
  const freqStr = matchStr(bare, "FREQ")?.toLowerCase();
  if (freqStr !== "daily" && freqStr !== "weekly" && freqStr !== "monthly") {
    return null;
  }
  const hour = matchInt(bare, "BYHOUR");
  const minute = matchInt(bare, "BYMINUTE") ?? 0;
  if (hour === null) return null;
  const byday = matchStr(bare, "BYDAY");
  const weekdays = byday ? byday.split(",").map((d) => d.toUpperCase()) : [];
  if (weekdays.some((d) => !WEEKDAYS.find((w) => w.token === d))) return null;
  const monthday = matchInt(bare, "BYMONTHDAY") ?? 1;
  return { freq: freqStr, weekdays, monthday, hour, minute };
}

function matchStr(bare: string, key: string): string | null {
  for (const attr of bare.split(";")) {
    const [k, v] = attr.split("=");
    if (k.toUpperCase() === key && v) return v;
  }
  return null;
}
