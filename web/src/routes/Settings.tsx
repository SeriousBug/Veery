import { useEffect, useState } from "react";
import { Switch } from "@ark-ui/react";
import { Loader2, Save, SlidersHorizontal, Timer, RefreshCw } from "lucide-react";
import { css } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";
import { http, HttpError } from "../api/http";
import { toaster } from "../lib/toaster";
import type { Settings as SettingsModel } from "../api/generated";

export function Settings() {
  const [form, setForm] = useState<SettingsModel | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    http
      .get<SettingsModel>("/api/settings")
      .then((data) => {
        if (!cancelled) setForm(data);
      })
      .catch((err) => {
        if (!cancelled)
          setLoadError(err instanceof HttpError ? err.message : "Could not load settings.");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function save() {
    if (!form) return;
    setSaving(true);
    try {
      await http.put("/api/settings", { ...form });
      toaster.create({ type: "success", title: "Settings saved", duration: 3000 });
    } catch (err) {
      toaster.create({
        type: "error",
        title: "Couldn't save settings",
        description: err instanceof HttpError ? err.message : "Please try again.",
      });
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className={vstack({ gap: "6", alignItems: "stretch" })}>
      <div>
        <h1 className={css({ fontSize: "3xl", fontWeight: "extrabold", letterSpacing: "-0.02em" })}>
          Settings
        </h1>
        <p className={css({ color: "textMuted", mt: "1" })}>
          Tune how often Veery checks in and how updates are handled.
        </p>
      </div>

      {loadError ? (
        <p className={css({ color: "coral.600", fontWeight: "bold" })}>{loadError}</p>
      ) : !form ? (
        <span className={hstack({ gap: "2", color: "textMuted" })}>
          <Loader2 size={18} className={css({ animation: "spin 0.9s linear infinite" })} />
          Loading settings…
        </span>
      ) : (
        <div
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
          <NumberField
            icon={<Timer size={18} className={css({ color: "teal.500" })} />}
            title="How often to check services"
            hint="Seconds between each refresh of service status and resource usage."
            suffix="seconds"
            min={1}
            value={form.pollIntervalSeconds}
            onChange={(v) => setForm({ ...form, pollIntervalSeconds: v })}
          />

          <ToggleField
            title="Auto-update new services by default"
            hint="Newly adopted services will keep themselves up to date unless you turn it off."
            checked={form.autoUpdateDefault}
            onChange={(v) => setForm({ ...form, autoUpdateDefault: v })}
          />

          <NumberField
            icon={<RefreshCw size={18} className={css({ color: "grape.500" })} />}
            title="How often to look for updates"
            hint="Minutes between checks for newer versions of your services."
            suffix="minutes"
            min={1}
            value={form.autoUpdateIntervalMinutes}
            onChange={(v) => setForm({ ...form, autoUpdateIntervalMinutes: v })}
          />

          <button
            onClick={save}
            disabled={saving}
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
            Save changes
          </button>
        </div>
      )}
    </div>
  );
}

function NumberField({
  icon,
  title,
  hint,
  suffix,
  value,
  min,
  onChange,
}: {
  icon: React.ReactNode;
  title: string;
  hint: string;
  suffix: string;
  value: number;
  min: number;
  onChange: (v: number) => void;
}) {
  return (
    <div className={vstack({ gap: "2", alignItems: "stretch" })}>
      <span className={hstack({ gap: "2.5" })}>
        {icon ?? <SlidersHorizontal size={18} />}
        <span className={css({ fontWeight: "extrabold", fontSize: "md" })}>{title}</span>
      </span>
      <span className={css({ fontSize: "sm", color: "textMuted" })}>{hint}</span>
      <span className={hstack({ gap: "2" })}>
        <input
          type="number"
          min={min}
          value={value}
          onChange={(e) => onChange(Math.max(min, Number(e.target.value) || min))}
          className={css({
            w: "28",
            px: "3.5",
            py: "2.5",
            borderRadius: "md",
            borderWidth: "1px",
            borderColor: "border",
            bg: "bg",
            fontWeight: "bold",
            fontSize: "md",
            color: "text",
            _focusVisible: { outline: "none", borderColor: "accent", boxShadow: "card" },
          })}
        />
        <span className={css({ color: "textMuted", fontWeight: "bold" })}>{suffix}</span>
      </span>
    </div>
  );
}

function ToggleField({
  title,
  hint,
  checked,
  onChange,
}: {
  title: string;
  hint: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <Switch.Root
      checked={checked}
      onCheckedChange={(d) => onChange(d.checked)}
      className={hstack({ justify: "space-between", gap: "4", cursor: "pointer" })}
    >
      <span className={vstack({ gap: "0.5", alignItems: "flex-start" })}>
        <Switch.Label className={css({ fontWeight: "extrabold", fontSize: "md", color: "text" })}>
          {title}
        </Switch.Label>
        <span className={css({ fontSize: "sm", color: "textMuted" })}>{hint}</span>
      </span>
      <Switch.Control
        className={css({
          w: "12",
          h: "7",
          borderRadius: "full",
          bg: "ink.200",
          p: "1",
          transition: "background 0.2s ease",
          flexShrink: 0,
          "&[data-state='checked']": { bg: "grape.500" },
        })}
      >
        <Switch.Thumb
          className={css({
            w: "5",
            h: "5",
            borderRadius: "full",
            bg: "white",
            boxShadow: "card",
            transition: "transform 0.2s ease",
            "&[data-state='checked']": { transform: "translateX(20px)" },
          })}
        />
      </Switch.Control>
      <Switch.HiddenInput />
    </Switch.Root>
  );
}
