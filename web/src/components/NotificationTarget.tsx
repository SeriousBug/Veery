import { useEffect, useState } from "react";
import { ChevronDown, ChevronRight, Trash2, ExternalLink } from "lucide-react";
import { css } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";
import { ToggleField } from "./ToggleField";
import { EVENTS } from "../lib/notificationEvents";
import {
  CUSTOM_SCHEME,
  SERVICES,
  buildURL,
  newTarget,
  serviceFor,
  type CompositeSpec,
  type FieldSpec,
  type Target,
} from "../lib/shoutrrr";
import type { NotificationEvent } from "../api/generated";

const OVERVIEW_DOCS = "https://containrrr.dev/shoutrrr/v0.8/services/overview/";

const inputStyle = css({
  px: "3",
  py: "2",
  borderRadius: "md",
  borderWidth: "1px",
  borderColor: "border",
  bg: "bg",
  color: "text",
  fontSize: "sm",
  w: "full",
  _focusVisible: { outline: "none", borderColor: "accent" },
  _disabled: { opacity: 0.7, cursor: "not-allowed" },
});

const invalidInputStyle = css({ borderColor: "coral.500" });

const labelStyle = css({ fontSize: "xs", fontWeight: "extrabold", color: "textMuted" });

function Field({
  spec,
  value,
  disabled,
  onChange,
}: {
  spec: FieldSpec;
  value: string;
  disabled?: boolean;
  onChange: (v: string) => void;
}) {
  const missing = Boolean(spec.required) && !value.trim();
  return (
    <label className={vstack({ gap: "1", alignItems: "stretch" })}>
      <span className={labelStyle}>
        {spec.label}
        {spec.required && <span className={css({ color: "coral.600" })}> *</span>}
      </span>
      {spec.options ? (
        <select
          value={value}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
          className={inputStyle}
        >
          {spec.options.map((o) => (
            <option key={o} value={o}>
              {o === "" ? "Default" : o}
            </option>
          ))}
        </select>
      ) : (
        <input
          type={spec.secret ? "password" : "text"}
          value={value}
          disabled={disabled}
          spellCheck={false}
          autoComplete="off"
          placeholder={spec.placeholder}
          onChange={(e) => onChange(e.target.value)}
          className={`${inputStyle} ${missing ? invalidInputStyle : ""}`}
        />
      )}
      {spec.hint && <span className={css({ fontSize: "xs", color: "textMuted" })}>{spec.hint}</span>}
    </label>
  );
}

/** One input that fills several fields, e.g. a Discord webhook URL. */
function CompositeField({
  spec,
  values,
  disabled,
  onChange,
}: {
  spec: CompositeSpec;
  values: Record<string, string>;
  disabled?: boolean;
  onChange: (patch: Record<string, string>) => void;
}) {
  const stored = spec.format(values);
  const [draft, setDraft] = useState(stored);

  // A change from elsewhere (a config reload, switching service) replaces the
  // draft, unless the draft already describes the same values. Deliberately not
  // keyed on the draft: reacting to what the user is typing would fight them.
  useEffect(() => {
    setDraft((current) =>
      stored && stored !== spec.format(spec.parse(current) ?? {}) ? stored : current,
    );
  }, [stored, spec]);

  const invalid = draft.trim() !== "" && spec.parse(draft) === null;
  const empty = draft.trim() === "";

  function edit(v: string) {
    setDraft(v);
    const parsed = spec.parse(v);
    onChange(parsed ?? Object.fromEntries(spec.covers.map((name) => [name, ""])));
  }

  return (
    <label className={vstack({ gap: "1", alignItems: "stretch" })}>
      <span className={labelStyle}>
        {spec.label}
        <span className={css({ color: "coral.600" })}> *</span>
      </span>
      <input
        value={draft}
        disabled={disabled}
        spellCheck={false}
        autoComplete="off"
        placeholder={spec.placeholder}
        onChange={(e) => edit(e.target.value)}
        className={`${inputStyle} ${invalid || empty ? invalidInputStyle : ""}`}
      />
      <span className={css({ fontSize: "xs", color: invalid ? "coral.600" : "textMuted" })}>
        {invalid ? spec.invalidHint : spec.hint}
      </span>
    </label>
  );
}

export function NotificationTarget({
  target,
  events,
  disabled,
  onChange,
  onEventsChange,
  onRemove,
}: {
  target: Target;
  events: Partial<Record<NotificationEvent, boolean>>;
  disabled?: boolean;
  onChange: (t: Target) => void;
  onEventsChange: (events: Partial<Record<NotificationEvent, boolean>>) => void;
  onRemove: () => void;
}) {
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [showEvents, setShowEvents] = useState(false);
  const spec = serviceFor(target.scheme);
  const url = buildURL(target);

  function setValue(name: string, v: string) {
    onChange({ ...target, values: { ...target.values, [name]: v } });
  }

  function setValues(patch: Record<string, string>) {
    onChange({ ...target, values: { ...target.values, ...patch } });
  }

  function setScheme(scheme: string) {
    onChange({ ...newTarget(scheme), id: target.id });
  }

  const covered = new Set(spec?.composite?.covers ?? []);
  const basic = spec?.fields.filter((f) => !f.advanced && !covered.has(f.name)) ?? [];
  const advanced = spec?.fields.filter((f) => f.advanced) ?? [];
  const enabledCount = EVENTS.filter(({ event }) => events[event] ?? true).length;

  return (
    <div
      className={vstack({
        gap: "3",
        alignItems: "stretch",
        p: "4",
        borderRadius: "lg",
        borderWidth: "1px",
        borderColor: "border",
        bg: "bg",
      })}
    >
      <div className={hstack({ gap: "3", justify: "space-between" })}>
        <span className={hstack({ gap: "2", flex: "1", minW: "0" })}>
          <select
            value={target.scheme}
            disabled={disabled}
            onChange={(e) => setScheme(e.target.value)}
            className={`${inputStyle} ${css({ maxW: "56", fontWeight: "extrabold" })}`}
          >
            {SERVICES.map((s) => (
              <option key={s.scheme} value={s.scheme}>
                {s.label}
              </option>
            ))}
            <option value={CUSTOM_SCHEME}>Other (paste an address)</option>
          </select>
        </span>
        {!disabled && (
          <button
            onClick={onRemove}
            title="Remove"
            className={hstack({
              gap: "1.5",
              px: "3",
              py: "2",
              borderRadius: "md",
              color: "coral.600",
              fontWeight: "extrabold",
              fontSize: "sm",
              cursor: "pointer",
              _hover: { bg: "coral.50" },
            })}
          >
            <Trash2 size={16} />
            Remove
          </button>
        )}
      </div>

      {target.scheme === CUSTOM_SCHEME ? (
        <label className={vstack({ gap: "1", alignItems: "stretch" })}>
          <span className={labelStyle}>Address</span>
          <input
            value={target.raw}
            disabled={disabled}
            spellCheck={false}
            autoComplete="off"
            placeholder="teams://group@tenant/altId/groupOwner?host=org.webhook.office.com"
            onChange={(e) => onChange({ ...target, raw: e.target.value })}
            className={`${inputStyle} ${css({ fontFamily: "mono" })}`}
          />
        </label>
      ) : (
        <>
          {spec?.composite && (
            <CompositeField
              spec={spec.composite}
              values={target.values}
              disabled={disabled}
              onChange={setValues}
            />
          )}

          <div
            className={css({
              display: "grid",
              gridTemplateColumns: { base: "1fr", md: "1fr 1fr" },
              gap: "3",
            })}
          >
            {basic.map((f) => (
              <Field
                key={f.name}
                spec={f}
                disabled={disabled}
                value={target.values[f.name] ?? ""}
                onChange={(v) => setValue(f.name, v)}
              />
            ))}
          </div>

          {advanced.length > 0 && (
            <div className={vstack({ gap: "3", alignItems: "stretch" })}>
              <button
                onClick={() => setShowAdvanced((s) => !s)}
                className={hstack({
                  gap: "1",
                  alignSelf: "flex-start",
                  fontSize: "sm",
                  fontWeight: "extrabold",
                  color: "textMuted",
                  cursor: "pointer",
                  _hover: { color: "accent" },
                })}
              >
                {showAdvanced ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
                Optional settings
              </button>
              {showAdvanced && (
                <div
                  className={css({
                    display: "grid",
                    gridTemplateColumns: { base: "1fr", md: "1fr 1fr" },
                    gap: "3",
                  })}
                >
                  {advanced.map((f) => (
                    <Field
                      key={f.name}
                      spec={f}
                      disabled={disabled}
                      value={target.values[f.name] ?? ""}
                      onChange={(v) => setValue(f.name, v)}
                    />
                  ))}
                </div>
              )}
            </div>
          )}
        </>
      )}

      <div className={vstack({ gap: "3", alignItems: "stretch" })}>
        <button
          onClick={() => setShowEvents((s) => !s)}
          className={hstack({
            gap: "1",
            alignSelf: "flex-start",
            fontSize: "sm",
            fontWeight: "extrabold",
            color: "textMuted",
            cursor: "pointer",
            _hover: { color: "accent" },
          })}
        >
          {showEvents ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          What to send here
          <span className={css({ fontWeight: "medium" })}>
            {enabledCount === EVENTS.length
              ? "(everything)"
              : `(${enabledCount} of ${EVENTS.length})`}
          </span>
        </button>
        {showEvents && (
          <div className={vstack({ gap: "4", alignItems: "stretch" })}>
            {EVENTS.map(({ event, title, hint }) => (
              <ToggleField
                key={event}
                title={title}
                hint={hint}
                disabled={disabled}
                checked={events[event] ?? true}
                onChange={(on) => onEventsChange({ ...events, [event]: on })}
              />
            ))}
          </div>
        )}
      </div>

      <div className={vstack({ gap: "1", alignItems: "flex-start" })}>
        {url && (
          <code
            className={css({
              fontFamily: "mono",
              fontSize: "xs",
              color: "textMuted",
              wordBreak: "break-all",
              pl: "3",
              borderLeftWidth: "2px",
              borderColor: "border",
            })}
          >
            {url}
          </code>
        )}
        <a
          href={spec?.docs ?? OVERVIEW_DOCS}
          target="_blank"
          rel="noreferrer"
          className={hstack({
            gap: "1",
            fontSize: "xs",
            color: "textMuted",
            _hover: { color: "accent", textDecoration: "underline" },
          })}
        >
          {spec ? `${spec.label} address format` : "All address formats"}
          <ExternalLink size={12} className={css({ flexShrink: 0 })} />
        </a>
      </div>
    </div>
  );
}
