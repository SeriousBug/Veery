import { useState } from "react";
import { Switch } from "@ark-ui/react";
import { Link } from "@tanstack/react-router";
import { AlertTriangle, RefreshCw } from "lucide-react";
import { css } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";
import { SourceAutomation, SourceUser, type Source } from "../api/generated";
import { useAuth } from "../auth/AuthProvider";
import { setAutoUpdate } from "../lib/actions";

export function AutoUpdateToggle({
  containerId,
  autoUpdate,
  source = SourceUser,
}: {
  containerId: string;
  autoUpdate: boolean;
  /** Who last set autoUpdate. Off because Veery gave up on the container reads
   * differently from off because the user chose to leave it off. */
  source?: Source;
}) {
  const [checked, setChecked] = useState(autoUpdate);
  const [pending, setPending] = useState(false);

  async function onChange(next: boolean) {
    setChecked(next);
    setPending(true);
    const ok = await setAutoUpdate(containerId, next);
    if (!ok) setChecked(!next);
    setPending(false);
  }

  // Flipping the switch makes the user the source server-side, so the warning
  // goes as soon as the switch does rather than waiting for a stacks refresh.
  const showStopped = !checked && source === SourceAutomation;

  return (
    <div className={vstack({ gap: "2", alignItems: "stretch" })}>
      <Switch.Root
        checked={checked}
        disabled={pending}
        onCheckedChange={(d) => onChange(d.checked)}
        className={hstack({
          justify: "space-between",
          gap: "4",
          p: "4",
          borderRadius: "lg",
          bg: showStopped ? "coral.50" : "grape.50",
          borderWidth: "1px",
          borderColor: showStopped ? "coral.100" : "grape.100",
          cursor: "pointer",
          _disabled: { opacity: 0.7, cursor: "not-allowed" },
        })}
      >
        <span className={hstack({ gap: "3" })}>
          <RefreshCw
            size={18}
            className={css({ color: showStopped ? "coral.500" : "grape.500" })}
          />
          <span className={vstack({ gap: "0", alignItems: "flex-start" })}>
            <Switch.Label
              className={css({ fontWeight: "extrabold", fontSize: "sm", color: "text" })}
            >
              Keep this up to date automatically
            </Switch.Label>
            <span className={css({ fontSize: "xs", color: "textMuted" })}>
              {showStopped
                ? "Veery turned this off. It's still running the version it's on."
                : "Veery installs new versions for you when they're ready."}
            </span>
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

      {showStopped && <StoppedNote />}
    </div>
  );
}

/** Why the toggle is off, when it was not the user who turned it off. Without
 * it the switch looks like a setting somebody chose, and the service quietly
 * stays behind. */
function StoppedNote() {
  // The event log is admin-only, so only an admin is offered the way into it.
  const { user } = useAuth();
  return (
    <p
      className={hstack({
        gap: "2",
        alignItems: "flex-start",
        px: "4",
        py: "3",
        borderRadius: "lg",
        bg: "coral.50",
        borderWidth: "1px",
        borderColor: "coral.100",
        fontSize: "xs",
        color: "textMuted",
        lineHeight: "1.5",
      })}
    >
      <AlertTriangle size={15} className={css({ color: "coral.500", flexShrink: 0, mt: "0.5" })} />
      <span>
        New versions kept failing to start and were rolled back, one after another, so Veery stopped
        trying instead of restarting this service every hour.{" "}
        {user?.isAdmin ? (
          <>
            The{" "}
            <Link
              to="/events"
              className={css({ color: "coral.600", fontWeight: "bold", textDecoration: "underline" })}
            >
              event log
            </Link>{" "}
            has what went wrong.{" "}
          </>
        ) : null}
        Turn this back on to start over once it's sorted.
      </span>
    </p>
  );
}
