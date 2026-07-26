import { useEffect, useState } from "react";
import { Bell, Loader2, Plus, Save, Send, Lock } from "lucide-react";
import { css } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";
import { http, HttpError } from "../api/http";
import { toaster } from "../lib/toaster";
import { NotificationTarget } from "./NotificationTarget";
import { buildURL, isComplete, newTarget, parseURL, type Target } from "../lib/shoutrrr";
import type {
  NotificationConfig,
  NotificationEvent,
  NotificationTarget as TargetConfig,
} from "../api/generated";

/** A target being edited: its address form plus the events it subscribes to. */
interface Row {
  target: Target;
  events: Partial<Record<NotificationEvent, boolean>>;
}

function rowFrom(t: TargetConfig): Row {
  return { target: parseURL(t.url), events: t.events ?? {} };
}

export function NotificationsSection() {
  const [cfg, setCfg] = useState<NotificationConfig | null>(null);
  const [rows, setRows] = useState<Row[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    http
      .get<NotificationConfig>("/api/notifications")
      .then((data) => {
        if (cancelled) return;
        setCfg(data);
        setRows(data.targets.map(rowFrom));
      })
      .catch((err) => {
        if (!cancelled)
          setLoadError(err instanceof HttpError ? err.message : "Could not load notifications.");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const complete = rows.filter((r) => isComplete(r.target));
  const urls = complete.map((r) => buildURL(r.target));
  const incomplete = rows.length - complete.length;
  const locked = cfg?.envManaged ?? false;

  function updateRow(id: string, patch: Partial<Row>) {
    setRows((rs) => rs.map((r) => (r.target.id === id ? { ...r, ...patch } : r)));
  }

  async function save() {
    if (!cfg) return;
    setSaving(true);
    try {
      const targets: TargetConfig[] = complete.map((r) => ({
        url: buildURL(r.target),
        events: r.events as TargetConfig["events"],
      }));
      const saved = await http.put<NotificationConfig>("/api/notifications", { targets });
      setCfg(saved);
      setRows(saved.targets.map(rowFrom));
      toaster.create({ type: "success", title: "Notifications saved", duration: 3000 });
    } catch (err) {
      toaster.create({
        type: "error",
        title: "Couldn't save notifications",
        description: err instanceof HttpError ? err.message : "Please try again.",
      });
    } finally {
      setSaving(false);
    }
  }

  // The test goes to whatever is typed in the box, so a channel can be checked
  // before it is saved.
  async function sendTest() {
    setTesting(true);
    try {
      await http.post("/api/notifications/test", { urls });
      toaster.create({
        type: "success",
        title: "Test sent",
        description: "Check the channel you configured.",
        duration: 4000,
      });
    } catch (err) {
      toaster.create({
        type: "error",
        title: "Test failed",
        description: err instanceof HttpError ? err.message : "Please try again.",
        duration: 8000,
      });
    } finally {
      setTesting(false);
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
          <Bell size={18} className={css({ color: "sunshine.500" })} />
          <span className={css({ fontWeight: "extrabold", fontSize: "md" })}>Notifications</span>
        </span>
      </div>

      {loadError ? (
        <p className={css({ color: "coral.600", fontWeight: "bold" })}>{loadError}</p>
      ) : !cfg ? (
        <span className={hstack({ gap: "2", color: "textMuted" })}>
          <Loader2 size={18} className={css({ animation: "spin 0.9s linear infinite" })} />
          Loading notifications…
        </span>
      ) : (
        <>
          {locked && (
            <span
              className={hstack({
                gap: "2",
                p: "3",
                borderRadius: "lg",
                bg: "bg",
                borderWidth: "1px",
                borderColor: "border",
                fontSize: "sm",
                color: "textMuted",
              })}
            >
              <Lock size={15} className={css({ flexShrink: 0 })} />
              Set by the VEERY_NOTIFY_URLS environment variable. Unset it to edit them here.
            </span>
          )}

          <div className={vstack({ gap: "3", alignItems: "stretch" })}>
            <span className={css({ fontWeight: "extrabold", fontSize: "md" })}>Where to send them</span>
            <span className={css({ fontSize: "sm", color: "textMuted" })}>
              Fields marked <span className={css({ color: "coral.600", fontWeight: "bold" })}>*</span>{" "}
              are required.
            </span>

            {rows.length === 0 && (
              <span className={css({ fontSize: "sm", color: "textMuted" })}>
                No places to send to yet.
              </span>
            )}

            {rows.map((r) => (
              <NotificationTarget
                key={r.target.id}
                target={r.target}
                events={r.events}
                disabled={locked}
                onChange={(target) => updateRow(r.target.id, { target })}
                onEventsChange={(events) => updateRow(r.target.id, { events })}
                onRemove={() => setRows((rs) => rs.filter((old) => old.target.id !== r.target.id))}
              />
            ))}

            {!locked && (
              <button
                onClick={() => setRows((rs) => [...rs, { target: newTarget("discord"), events: {} }])}
                className={hstack({
                  gap: "2",
                  alignSelf: "flex-start",
                  px: "4",
                  py: "2.5",
                  borderRadius: "full",
                  bg: "bg",
                  color: "text",
                  borderWidth: "1px",
                  borderStyle: "dashed",
                  borderColor: "border",
                  fontWeight: "extrabold",
                  fontSize: "sm",
                  cursor: "pointer",
                  _hover: { borderColor: "accent", color: "accent" },
                })}
              >
                <Plus size={16} />
                Add a place
              </button>
            )}
          </div>

          <div className={hstack({ gap: "3", flexWrap: "wrap", alignItems: "center" })}>
            {!locked && (
              <button
                onClick={save}
                disabled={saving || incomplete > 0}
                className={hstack({
                  gap: "2",
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
            )}
            <button
              onClick={sendTest}
              disabled={testing || urls.length === 0}
              className={hstack({
                gap: "2",
                px: "5",
                py: "3",
                borderRadius: "full",
                bg: "bg",
                color: "text",
                borderWidth: "1px",
                borderColor: "border",
                fontWeight: "extrabold",
                cursor: "pointer",
                _hover: { borderColor: "accent" },
                _disabled: { opacity: 0.6, cursor: "not-allowed" },
              })}
            >
              {testing ? (
                <Loader2 size={18} className={css({ animation: "spin 0.9s linear infinite" })} />
              ) : (
                <Send size={18} />
              )}
              Send a test
            </button>
            {incomplete > 0 && (
              <span className={css({ fontSize: "sm", color: "coral.600", fontWeight: "bold" })}>
                {incomplete === 1
                  ? "One place is missing a required field."
                  : `${incomplete} places are missing required fields.`}
              </span>
            )}
          </div>
        </>
      )}
    </section>
  );
}
