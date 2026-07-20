import { useEffect, useState } from "react";
import { Switch } from "@ark-ui/react";
import { startRegistration } from "@simplewebauthn/browser";
import type { PublicKeyCredentialCreationOptionsJSON } from "@simplewebauthn/browser";
import {
  Loader2,
  Save,
  SlidersHorizontal,
  Timer,
  RefreshCw,
  KeyRound,
  PlusCircle,
  HardDrive,
  Activity,
  ScrollText,
} from "lucide-react";
import { css } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";
import { http, HttpError } from "../api/http";
import { toaster } from "../lib/toaster";
import { ToggleField } from "../components/ToggleField";
import { NotificationsSection } from "../components/NotificationsSection";
import { useAuth } from "../auth/AuthProvider";
import { formatAge } from "../lib/format";
import type { Settings as SettingsModel, DiskItem } from "../api/generated";

export function Settings() {
  const { user } = useAuth();
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
            hint="Services you add will keep themselves up to date unless you turn it off."
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

          <NumberField
            icon={<ScrollText size={18} className={css({ color: "teal.500" })} />}
            title="How long to keep the event log"
            hint="Days of history to keep on the Events page. Older events are removed. Set to 0 to keep everything."
            suffix="days"
            min={0}
            value={form.eventLogRetentionDays}
            onChange={(v) => setForm({ ...form, eventLogRetentionDays: v })}
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

      <DiskVisibilitySection />

      {user?.isAdmin && <NotificationsSection />}

      <PasskeysSection />
    </div>
  );
}

function DiskVisibilitySection() {
  const [items, setItems] = useState<DiskItem[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [savingKey, setSavingKey] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    http
      .get<DiskItem[]>("/api/disks")
      .then((data) => {
        if (!cancelled) setItems(data);
      })
      .catch((err) => {
        if (!cancelled)
          setLoadError(err instanceof HttpError ? err.message : "Could not load disks.");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function toggle(key: string, shown: boolean) {
    if (!items) return;
    const next = items.map((it) => (it.key === key ? { ...it, shown } : it));
    setItems(next);
    setSavingKey(key);
    try {
      const visibility: Record<string, boolean> = {};
      for (const it of next) visibility[it.key] = it.shown;
      const updated = await http.put<DiskItem[]>("/api/disks", { visibility });
      setItems(updated);
    } catch (err) {
      setItems(items);
      toaster.create({
        type: "error",
        title: "Couldn't update disks",
        description: err instanceof HttpError ? err.message : "Please try again.",
      });
    } finally {
      setSavingKey(null);
    }
  }

  const capacity = items?.filter((it) => it.kind === "capacity") ?? [];
  const activity = items?.filter((it) => it.kind === "activity") ?? [];

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
        <span className={hstack({ gap: "2.5" })}>
          <HardDrive size={18} className={css({ color: "teal.500" })} />
          <span className={css({ fontWeight: "extrabold", fontSize: "md" })}>Which disks to show</span>
        </span>
        <span className={css({ fontSize: "sm", color: "textMuted" })}>
          Choose the drives that appear on the dashboard. Applies to everyone.
        </span>
      </div>

      {loadError ? (
        <p className={css({ color: "coral.600", fontWeight: "bold" })}>{loadError}</p>
      ) : !items ? (
        <span className={hstack({ gap: "2", color: "textMuted" })}>
          <Loader2 size={18} className={css({ animation: "spin 0.9s linear infinite" })} />
          Loading disks…
        </span>
      ) : items.length === 0 ? (
        <p className={css({ color: "textMuted" })}>No disks detected on this machine.</p>
      ) : (
        <div className={vstack({ gap: "5", alignItems: "stretch" })}>
          <DiskGroup
            icon={<HardDrive size={15} className={css({ color: "textMuted" })} />}
            title="Storage"
            items={capacity}
            savingKey={savingKey}
            onToggle={toggle}
          />
          <DiskGroup
            icon={<Activity size={15} className={css({ color: "textMuted" })} />}
            title="Disk activity"
            items={activity}
            savingKey={savingKey}
            onToggle={toggle}
          />
        </div>
      )}
    </section>
  );
}

function DiskGroup({
  icon,
  title,
  items,
  savingKey,
  onToggle,
}: {
  icon: React.ReactNode;
  title: string;
  items: DiskItem[];
  savingKey: string | null;
  onToggle: (key: string, shown: boolean) => void;
}) {
  if (items.length === 0) return null;
  return (
    <div className={vstack({ gap: "2.5", alignItems: "stretch" })}>
      <span className={hstack({ gap: "1.5", fontSize: "sm", fontWeight: "extrabold", color: "textMuted" })}>
        {icon}
        {title}
      </span>
      {items.map((it) => (
        <Switch.Root
          key={it.key}
          checked={it.shown}
          disabled={savingKey === it.key}
          onCheckedChange={(d) => onToggle(it.key, d.checked)}
          className={hstack({
            justify: "space-between",
            gap: "4",
            p: "3.5",
            borderRadius: "lg",
            bg: "bg",
            borderWidth: "1px",
            borderColor: "border",
            cursor: "pointer",
            _disabled: { opacity: 0.7, cursor: "not-allowed" },
          })}
        >
          <span className={vstack({ gap: "0", alignItems: "flex-start", minW: "0" })}>
            <Switch.Label className={css({ fontWeight: "bold", color: "text" })}>
              {it.label}
            </Switch.Label>
            <span
              className={css({
                fontSize: "xs",
                color: "textMuted",
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
                maxW: "full",
              })}
            >
              {it.detail}
            </span>
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
                display: "block",
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
      ))}
    </div>
  );
}

interface DeviceRegistrationOptions {
  publicKey: PublicKeyCredentialCreationOptionsJSON;
}

function PasskeysSection() {
  const { credentials, refresh } = useAuth();
  const [busy, setBusy] = useState(false);

  async function addDevice() {
    setBusy(true);
    try {
      const options = await http.post<DeviceRegistrationOptions>(
        "/auth/register/device/begin",
      );
      const credential = await startRegistration({ optionsJSON: options.publicKey });
      await http.post(
        "/auth/register/device/finish",
        credential as unknown as Record<string, unknown>,
      );
      await refresh();
      toaster.create({
        type: "success",
        title: "New device added",
        description: "You can now sign in with it.",
        duration: 4000,
      });
    } catch (err) {
      if (err instanceof DOMException && err.name === "NotAllowedError") {
        // User cancelled the browser prompt; stay quiet.
      } else {
        toaster.create({
          type: "error",
          title: "Couldn't add that device",
          description:
            err instanceof HttpError ? err.message : "Please try again.",
        });
      }
    } finally {
      setBusy(false);
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
        <span className={hstack({ gap: "2.5" })}>
          <KeyRound size={18} className={css({ color: "grape.500" })} />
          <span className={css({ fontWeight: "extrabold", fontSize: "md" })}>
            Your devices
          </span>
        </span>
        <span className={css({ fontSize: "sm", color: "textMuted" })}>
          Add your other phone or a backup key so you never get locked out.
        </span>
      </div>

      {credentials.length > 0 && (
        <div className={vstack({ gap: "2.5", alignItems: "stretch" })}>
          {credentials.map((cred) => (
            <div
              key={cred.id}
              className={hstack({
                justify: "space-between",
                gap: "3",
                flexWrap: "wrap",
                p: "3.5",
                borderRadius: "lg",
                bg: "bg",
                borderWidth: "1px",
                borderColor: "border",
              })}
            >
              <span className={hstack({ gap: "2.5", minW: "0" })}>
                <KeyRound size={16} className={css({ color: "teal.500" })} />
                <span className={css({ fontWeight: "bold", color: "text" })}>{cred.name}</span>
              </span>
              <span className={css({ fontSize: "sm", color: "textMuted" })}>
                Added {formatAge(cred.createdAt)}
              </span>
            </div>
          ))}
        </div>
      )}

      <button
        onClick={addDevice}
        disabled={busy}
        className={hstack({
          gap: "2",
          alignSelf: "flex-start",
          px: "5",
          py: "2.5",
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
        {busy ? (
          <Loader2 size={18} className={css({ animation: "spin 0.9s linear infinite" })} />
        ) : (
          <PlusCircle size={18} />
        )}
        Add another device
      </button>
    </section>
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
