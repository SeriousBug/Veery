import { useEffect, useState } from "react";
import { Bell, Loader2, Save, Send, Lock } from "lucide-react";
import { css } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";
import { http, HttpError } from "../api/http";
import { toaster } from "../lib/toaster";
import { ToggleField } from "./ToggleField";
import type { NotificationConfig, NotificationEvent } from "../api/generated";

const EVENTS: { event: NotificationEvent; title: string; hint: string }[] = [
  {
    event: "container_status",
    title: "Service problems",
    hint: "A service you manage crashes, goes unhealthy, disappears, stops, or comes back up.",
  },
  {
    event: "update_applied",
    title: "Update results",
    hint: "An update finished, or failed and was rolled back.",
  },
  {
    event: "update_available",
    title: "Updates you can install",
    hint: "A newer version is out for a service that doesn't update itself.",
  },
  {
    event: "auth",
    title: "Sign-ins and passkeys",
    hint: "Someone signs in, or a new passkey is enrolled.",
  },
];

const PLACEHOLDER = `discord://token@channel-id
ntfy://ntfy.sh/my-topic
slack://hook:token-a/token-b/token-c`;

export function NotificationsSection() {
  const [cfg, setCfg] = useState<NotificationConfig | null>(null);
  const [urlText, setUrlText] = useState("");
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
        setUrlText(data.urls.join("\n"));
      })
      .catch((err) => {
        if (!cancelled)
          setLoadError(err instanceof HttpError ? err.message : "Could not load notifications.");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const urls = urlText
    .split("\n")
    .map((u) => u.trim())
    .filter(Boolean);
  const locked = cfg?.envManaged ?? false;

  function setEvent(event: NotificationEvent, on: boolean) {
    if (!cfg) return;
    setCfg({ ...cfg, events: { ...cfg.events, [event]: on } });
  }

  async function save() {
    if (!cfg) return;
    setSaving(true);
    try {
      const saved = await http.put<NotificationConfig>("/api/notifications", {
        urls,
        events: cfg.events,
      });
      setCfg(saved);
      setUrlText(saved.urls.join("\n"));
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
        <span className={css({ fontSize: "sm", color: "textMuted" })}>
          Tell Veery where to reach you when something happens.
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

          <div className={vstack({ gap: "2", alignItems: "stretch" })}>
            <span className={css({ fontWeight: "extrabold", fontSize: "md" })}>Where to send them</span>
            <span className={css({ fontSize: "sm", color: "textMuted" })}>
              One address per line. Discord, ntfy, Slack, Telegram, Gotify, Pushover, Matrix, email
              and plain webhooks all work — see{" "}
              <a
                href="https://containrrr.dev/shoutrrr/v0.8/services/overview/"
                target="_blank"
                rel="noreferrer"
                className={css({ color: "accent", fontWeight: "bold", textDecoration: "underline" })}
              >
                the list of address formats
              </a>
              .
            </span>
            <textarea
              value={urlText}
              disabled={locked}
              spellCheck={false}
              rows={Math.max(3, urls.length + 1)}
              placeholder={PLACEHOLDER}
              onChange={(e) => setUrlText(e.target.value)}
              className={css({
                px: "3.5",
                py: "2.5",
                borderRadius: "md",
                borderWidth: "1px",
                borderColor: "border",
                bg: "bg",
                color: "text",
                fontFamily: "mono",
                fontSize: "sm",
                resize: "vertical",
                _focusVisible: { outline: "none", borderColor: "accent", boxShadow: "card" },
                _disabled: { opacity: 0.7, cursor: "not-allowed" },
              })}
            />
          </div>

          <div className={vstack({ gap: "4", alignItems: "stretch" })}>
            <span className={css({ fontWeight: "extrabold", fontSize: "md" })}>What to tell you about</span>
            {EVENTS.map(({ event, title, hint }) => (
              <ToggleField
                key={event}
                title={title}
                hint={hint}
                disabled={locked}
                checked={cfg.events[event] ?? true}
                onChange={(on) => setEvent(event, on)}
              />
            ))}
          </div>

          <div className={hstack({ gap: "3", flexWrap: "wrap" })}>
            {!locked && (
              <button
                onClick={save}
                disabled={saving}
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
          </div>
        </>
      )}
    </section>
  );
}
