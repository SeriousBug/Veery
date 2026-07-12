import { useState } from "react";
import { ChevronDown, ChevronRight, Trash2, ExternalLink } from "lucide-react";
import { css } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";
import {
  CUSTOM_SCHEME,
  SERVICES,
  buildURL,
  newTarget,
  serviceFor,
  type FieldSpec,
  type Target,
} from "../lib/shoutrrr";

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

export function NotificationTarget({
  target,
  disabled,
  onChange,
  onRemove,
}: {
  target: Target;
  disabled?: boolean;
  onChange: (t: Target) => void;
  onRemove: () => void;
}) {
  const [showAdvanced, setShowAdvanced] = useState(false);
  const spec = serviceFor(target.scheme);
  const url = buildURL(target);

  function setValue(name: string, v: string) {
    onChange({ ...target, values: { ...target.values, [name]: v } });
  }

  function setScheme(scheme: string) {
    onChange({ ...newTarget(scheme), id: target.id });
  }

  const basic = spec?.fields.filter((f) => !f.advanced) ?? [];
  const advanced = spec?.fields.filter((f) => f.advanced) ?? [];

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
