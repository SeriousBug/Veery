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

// describeRRule returns a human sentence for a rule, including the time of day.
// toText() renders BYHOUR as a bare "at 1", so the time parts are dropped before
// handing it the rule and a formatted clock time is appended instead. Returns
// null when the rule is unparseable.
export function describeRRule(rule: string): string | null {
  const bare = stripPrefix(rule);
  if (!bare) return null;
  let text: string;
  try {
    text = RRule.fromString(withoutTime(bare)).toText();
  } catch {
    return null;
  }
  const time = timeFromRRule(bare);
  return time ? `${text} at ${time}` : text;
}

function withoutTime(bare: string): string {
  return bare
    .split(";")
    .filter((attr) => !/^(BYHOUR|BYMINUTE|BYSECOND)=/i.test(attr.trim()))
    .join(";");
}

/**
 * The zone the server evaluates schedules in, from `GET /api/mdadm/schedules`.
 * The name is empty when the server could not determine it, and the offset then
 * stands in for it.
 */
export interface ServerZone {
  timeZone: string;
  offsetSeconds: number;
}

// DTSTART the server uses for rules that carry none, mirroring scheduleAnchor in
// internal/raidwatch/schedule.go. It has to match, or a rule with an INTERVAL
// would be previewed on a different phase than it actually runs on.
const SCHEDULE_ANCHOR = new Date(Date.UTC(2020, 0, 1));

// nextRun returns the instant a rule next fires, evaluated in the server's
// timezone rather than the browser's: the scrub runs on the server, so a browser
// set to another zone still has to be told the right answer. The rule is walked
// in naive wall-clock time, which is what rrule-go does in the server's
// time.Local, and only the result is turned into an instant. Returns null when
// the rule is unparseable or never fires again.
export function nextRun(rule: string, zone: ServerZone, from: Date = new Date()): Date | null {
  const bare = stripPrefix(rule);
  if (!bare) return null;
  try {
    const options = RRule.parseString(bare);
    if (!options.dtstart) options.dtstart = SCHEDULE_ANCHOR;
    const wallNow = new Date(from.getTime() + offsetAt(zone, from) * 1000);
    const next = new RRule(options).after(wallNow);
    return next ? instantOf(next, zone) : null;
  } catch {
    return null;
  }
}

// offsetAt is the server zone's UTC offset in seconds at a given instant, which
// is what makes the answer survive a DST change between now and the next run.
function offsetAt(zone: ServerZone, at: Date): number {
  if (!zone.timeZone) return zone.offsetSeconds;
  try {
    const parts = new Intl.DateTimeFormat("en-US", {
      timeZone: zone.timeZone,
      hourCycle: "h23",
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    }).formatToParts(at);
    const get = (type: string) => Number(parts.find((p) => p.type === type)?.value);
    const asUTC = Date.UTC(get("year"), get("month") - 1, get("day"), get("hour"), get("minute"));
    return Math.round((asUTC - at.getTime()) / 60_000) * 60;
  } catch {
    return zone.offsetSeconds;
  }
}

// instantOf turns a wall-clock time in the server's zone (held as if it were
// UTC) into the instant it names. The offset that applies is the one at the
// instant itself, so the first guess is refined once; a second pass settles the
// case where the guess landed on the other side of a DST change.
function instantOf(wall: Date, zone: ServerZone): Date {
  let guess = new Date(wall.getTime() - offsetAt(zone, wall) * 1000);
  for (let i = 0; i < 2; i++) {
    const refined = new Date(wall.getTime() - offsetAt(zone, guess) * 1000);
    if (refined.getTime() === guess.getTime()) break;
    guess = refined;
  }
  return guess;
}

// formatUntil renders the wait until a future date, e.g. "in about 3 hours".
export function formatUntil(target: Date, from: Date = new Date()): string {
  const minutes = Math.round((target.getTime() - from.getTime()) / 60_000);
  if (minutes <= 1) return "in a moment";
  if (minutes < 60) return `in ${minutes} minutes`;
  const hours = Math.round(minutes / 60);
  if (hours < 48) return `in about ${hours} ${hours === 1 ? "hour" : "hours"}`;
  const days = Math.round(hours / 24);
  return `in about ${days} days`;
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
