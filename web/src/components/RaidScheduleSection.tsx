import { useEffect, useState } from "react";
import { CalendarClock, Loader2, Save } from "lucide-react";
import { css, cx } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";
import { http, HttpError } from "../api/http";
import { toaster } from "../lib/toaster";
import { useLiveData } from "../live/LiveData";
import { ToggleField } from "./ToggleField";
import {
  buildRRule,
  defaultBuilder,
  describeRRule,
  parseBuilder,
  WEEKDAYS,
  type Builder,
  type Freq,
} from "../lib/rrule";
import type { MdadmScheduleConfig, MdadmSchedule } from "../api/generated";

interface Form {
  enabled: boolean;
  useRaw: boolean;
  builder: Builder;
  raw: string;
}

function formFromSchedule(sc: MdadmSchedule | undefined): Form {
  if (!sc || !sc.rrule) {
    return { enabled: false, useRaw: false, builder: { ...defaultBuilder }, raw: "" };
  }
  const parsed = parseBuilder(sc.rrule);
  return {
    enabled: sc.enabled,
    useRaw: parsed === null,
    builder: parsed ?? { ...defaultBuilder },
    raw: sc.rrule,
  };
}

// ruleOf is the effective RRULE a form will save.
function ruleOf(f: Form): string {
  return f.useRaw ? f.raw.trim() : buildRRule(f.builder);
}

export function RaidScheduleSection() {
  const { metrics } = useLiveData();
  const arrays = metrics?.host.mdadm;
  const [forms, setForms] = useState<Record<string, Form>>({});
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    http
      .get<MdadmScheduleConfig>("/api/mdadm/schedules")
      .then((cfg) => {
        if (cancelled) return;
        const next: Record<string, Form> = {};
        for (const [name, sc] of Object.entries(cfg.schedules ?? {})) {
          next[name] = formFromSchedule(sc);
        }
        setForms(next);
        setLoaded(true);
      })
      .catch(() => {
        if (!cancelled) setLoaded(true);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (!arrays || arrays.length === 0) return null;

  function formFor(name: string): Form {
    return forms[name] ?? formFromSchedule(undefined);
  }

  function update(name: string, patch: Partial<Form>) {
    setForms((prev) => ({ ...prev, [name]: { ...formFor(name), ...patch } }));
  }

  async function save() {
    const schedules: Record<string, MdadmSchedule> = {};
    for (const a of arrays ?? []) {
      const f = forms[a.name];
      if (!f) continue;
      const rule = ruleOf(f);
      if (f.enabled && rule === "") {
        toaster.create({
          type: "error",
          title: `${a.name}: schedule is empty`,
          description: "Enter a schedule or turn it off.",
        });
        return;
      }
      if (rule !== "") schedules[a.name] = { rrule: rule, enabled: f.enabled };
    }
    setSaving(true);
    try {
      const cfg = await http.put<MdadmScheduleConfig>("/api/mdadm/schedules", { schedules });
      const next: Record<string, Form> = {};
      for (const [name, sc] of Object.entries(cfg.schedules ?? {})) {
        next[name] = formFromSchedule(sc);
      }
      setForms(next);
      toaster.create({ type: "success", title: "Scan schedules saved", duration: 3000 });
    } catch (err) {
      toaster.create({
        type: "error",
        title: "Couldn't save schedules",
        description: err instanceof HttpError ? err.message : "Please try again.",
      });
    } finally {
      setSaving(false);
    }
  }

  return (
    <section
      className={vstack({
        gap: "5",
        alignItems: "stretch",
        p: "6",
        borderRadius: "xl",
        bg: "surface",
        borderWidth: "1px",
        borderColor: "border",
        boxShadow: "card",
      })}
    >
      <div className={vstack({ gap: "1", alignItems: "flex-start" })}>
        <div className={hstack({ gap: "2.5" })}>
          <CalendarClock size={20} className={css({ color: "grape.500" })} />
          <h2 className={css({ fontSize: "lg", fontWeight: "extrabold" })}>Automatic RAID scans</h2>
        </div>
        <p className={css({ fontSize: "sm", color: "textMuted" })}>
          Schedule periodic data-scrubs per array. Times use this server's timezone. A scrub is
          skipped if the array is already busy.
        </p>
      </div>

      {loaded &&
        arrays.map((a) => (
          <ArrayScheduleRow
            key={a.name}
            name={a.name}
            level={a.level}
            form={formFor(a.name)}
            onChange={(patch) => update(a.name, patch)}
          />
        ))}

      <button
        onClick={save}
        disabled={saving || !loaded}
        className={hstack({
          gap: "2",
          alignSelf: "flex-start",
          px: "6",
          py: "3",
          borderRadius: "full",
          bg: "accent",
          color: "white",
          fontWeight: "extrabold",
          cursor: "pointer",
          boxShadow: "card",
          _hover: { bg: "accentHover" },
          _disabled: { opacity: 0.6, cursor: "not-allowed" },
        })}
      >
        {saving ? (
          <Loader2 size={18} className={css({ animation: "spin 0.9s linear infinite" })} />
        ) : (
          <Save size={18} />
        )}
        Save schedules
      </button>
    </section>
  );
}

function ArrayScheduleRow({
  name,
  level,
  form,
  onChange,
}: {
  name: string;
  level: string;
  form: Form;
  onChange: (patch: Partial<Form>) => void;
}) {
  const rule = ruleOf(form);
  const description = describeRRule(rule);

  return (
    <div
      className={vstack({
        gap: "3",
        alignItems: "stretch",
        p: "4",
        borderRadius: "lg",
        borderWidth: "1px",
        borderColor: "border",
      })}
    >
      <ToggleField
        title={name}
        hint={`${level} — scrub on a schedule`}
        checked={form.enabled}
        onChange={(enabled) => onChange({ enabled })}
      />

      {form.enabled && (
        <div className={vstack({ gap: "3", alignItems: "stretch" })}>
          {form.useRaw ? (
            <RawEditor value={form.raw} onChange={(raw) => onChange({ raw })} />
          ) : (
            <BuilderEditor builder={form.builder} onChange={(builder) => onChange({ builder })} />
          )}

          <div className={hstack({ justify: "space-between", gap: "3", flexWrap: "wrap" })}>
            <span className={css({ fontSize: "sm", color: description ? "textMuted" : "coral.500" })}>
              {description ? `Runs ${description}` : "Invalid schedule"}
            </span>
            <button
              type="button"
              onClick={() => onChange({ useRaw: !form.useRaw, raw: form.useRaw ? form.raw : rule })}
              className={css({
                fontSize: "sm",
                fontWeight: "bold",
                color: "accent",
                cursor: "pointer",
                _hover: { textDecoration: "underline" },
              })}
            >
              {form.useRaw ? "Use the builder" : "Edit raw RRULE"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

const FREQS: { value: Freq; label: string }[] = [
  { value: "daily", label: "Daily" },
  { value: "weekly", label: "Weekly" },
  { value: "monthly", label: "Monthly" },
];

function BuilderEditor({
  builder,
  onChange,
}: {
  builder: Builder;
  onChange: (b: Builder) => void;
}) {
  const timeValue = `${pad(builder.hour)}:${pad(builder.minute)}`;

  return (
    <div className={vstack({ gap: "3", alignItems: "stretch" })}>
      <div className={hstack({ gap: "3", flexWrap: "wrap" })}>
        <label className={fieldLabel}>
          Frequency
          <select
            value={builder.freq}
            onChange={(e) => onChange({ ...builder, freq: e.target.value as Freq })}
            className={inputBox}
          >
            {FREQS.map((f) => (
              <option key={f.value} value={f.value}>
                {f.label}
              </option>
            ))}
          </select>
        </label>

        <label className={fieldLabel}>
          Time
          <input
            type="time"
            value={timeValue}
            onChange={(e) => {
              const [h, m] = e.target.value.split(":").map((n) => parseInt(n, 10));
              onChange({ ...builder, hour: h || 0, minute: m || 0 });
            }}
            className={inputBox}
          />
        </label>

        {builder.freq === "monthly" && (
          <label className={fieldLabel}>
            Day of month
            <input
              type="number"
              min={1}
              max={31}
              value={builder.monthday}
              onChange={(e) =>
                onChange({ ...builder, monthday: clampDay(parseInt(e.target.value, 10)) })
              }
              className={inputBox}
            />
          </label>
        )}
      </div>

      {builder.freq === "weekly" && (
        <div className={hstack({ gap: "1.5", flexWrap: "wrap" })}>
          {WEEKDAYS.map((d) => {
            const on = builder.weekdays.includes(d.token);
            return (
              <button
                key={d.token}
                type="button"
                title={d.label}
                onClick={() => {
                  const weekdays = on
                    ? builder.weekdays.filter((t) => t !== d.token)
                    : [...builder.weekdays, d.token];
                  onChange({ ...builder, weekdays });
                }}
                className={cx(
                  css({
                    w: "9",
                    h: "9",
                    borderRadius: "full",
                    fontSize: "sm",
                    fontWeight: "bold",
                    cursor: "pointer",
                    borderWidth: "1px",
                    borderColor: "border",
                  }),
                  css(on ? { bg: "grape.500", color: "white" } : { bg: "ink.100", color: "text" }),
                )}
              >
                {d.short}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

function RawEditor({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <label className={vstack({ gap: "1", alignItems: "stretch" })}>
      <span className={css({ fontSize: "sm", fontWeight: "bold", color: "textMuted" })}>
        RRULE (RFC 5545)
      </span>
      <input
        type="text"
        value={value}
        spellCheck={false}
        placeholder="FREQ=WEEKLY;BYDAY=SU;BYHOUR=20;BYMINUTE=0"
        onChange={(e) => onChange(e.target.value)}
        className={cx(inputBox, css({ fontFamily: "mono", w: "full" }))}
      />
    </label>
  );
}

const fieldLabel = vstack({
  gap: "1",
  alignItems: "flex-start",
  fontSize: "sm",
  fontWeight: "bold",
  color: "textMuted",
});

const inputBox = css({
  px: "3",
  py: "2",
  borderRadius: "md",
  borderWidth: "1px",
  borderColor: "border",
  bg: "surface",
  color: "text",
  fontSize: "sm",
  fontWeight: "medium",
});

function pad(n: number): string {
  return n.toString().padStart(2, "0");
}

function clampDay(n: number): number {
  if (Number.isNaN(n)) return 1;
  return Math.min(31, Math.max(1, n));
}
