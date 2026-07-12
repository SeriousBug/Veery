import { useState, type ReactNode } from "react";
import {
  Play,
  Square,
  RotateCw,
  ArrowUpCircle,
  LifeBuoy,
  Loader2,
  CheckCircle2,
} from "lucide-react";
import { css, cx } from "styled-system/css";
import { hstack } from "styled-system/patterns";
import { ConfirmDialog } from "./ConfirmDialog";
import type { ContainerStatus, JobProgress } from "../api/generated";

type Variant = "primary" | "secondary" | "danger" | "update";

const VARIANT: Record<Variant, string> = {
  primary: css({ bg: "accent", color: "white", _hover: { bg: "accentHover" }, boxShadow: "card" }),
  secondary: css({ bg: "ink.100", color: "text", _hover: { bg: "ink.200" } }),
  danger: css({ bg: "coral.300", color: "ink.900", _hover: { bg: "coral.400" } }),
  update: css({ bg: "sunshine.400", color: "ink.900", _hover: { bg: "sunshine.500" }, boxShadow: "card" }),
};

function btn(variant: Variant, size: "sm" | "md") {
  return cx(
    hstack({
      gap: "1.5",
      justify: "center",
      px: size === "sm" ? "3.5" : "4",
      py: size === "sm" ? "2" : "2.5",
      borderRadius: "full",
      fontWeight: "extrabold",
      fontSize: "sm",
      cursor: "pointer",
      transition: "all 0.15s ease",
      whiteSpace: "nowrap",
      _disabled: { opacity: 0.6, cursor: "not-allowed" },
    }),
    VARIANT[variant],
  );
}

export interface ActionHandlers {
  onStart: () => void;
  onStop: () => void;
  onRestart: () => void;
  onUpdate: () => void;
  onBringUp: () => void;
}

export function ActionBar({
  name,
  status,
  busy,
  updateAvailable,
  showUpdate,
  updateImage,
  size = "md",
  handlers,
}: {
  name: string;
  status: ContainerStatus;
  busy?: JobProgress;
  updateAvailable?: boolean;
  /** Show an update affordance even when no update is available (managed services). */
  showUpdate?: boolean;
  /** Current image/tag, shown in the update confirmation for power users. */
  updateImage?: string;
  size?: "sm" | "md";
  handlers: ActionHandlers;
}) {
  const [confirmStop, setConfirmStop] = useState(false);
  const [confirmUpdate, setConfirmUpdate] = useState(false);

  if (busy) {
    return (
      <span
        className={hstack({
          gap: "2",
          px: "4",
          py: size === "sm" ? "2" : "2.5",
          borderRadius: "full",
          bg: "grape.100",
          color: "grape.700",
          fontWeight: "extrabold",
          fontSize: "sm",
        })}
      >
        <Loader2 size={16} className={css({ animation: "spin 0.9s linear infinite" })} />
        {busy.message || "Working…"}
      </span>
    );
  }

  const buttons: ReactNode[] = [];

  if (status === "missing") {
    buttons.push(
      <button key="bringup" className={btn("primary", size)} onClick={handlers.onBringUp}>
        <LifeBuoy size={16} /> Bring back up
      </button>,
    );
  } else if (status === "running") {
    buttons.push(
      <button key="restart" className={btn("primary", size)} onClick={handlers.onRestart}>
        <RotateCw size={16} /> Restart
      </button>,
      <button key="stop" className={btn("secondary", size)} onClick={() => setConfirmStop(true)}>
        <Square size={16} /> Stop
      </button>,
    );
  } else {
    // stopped / needs_attention / unknown
    buttons.push(
      <button key="start" className={btn("primary", size)} onClick={handlers.onStart}>
        <Play size={16} /> Start
      </button>,
    );
    if (status === "needs_attention") {
      buttons.push(
        <button key="restart" className={btn("secondary", size)} onClick={handlers.onRestart}>
          <RotateCw size={16} /> Restart
        </button>,
      );
    }
  }

  if (updateAvailable) {
    buttons.push(
      <button
        key="update"
        className={btn("update", size)}
        onClick={() => setConfirmUpdate(true)}
      >
        <ArrowUpCircle size={16} /> Update now
      </button>,
    );
  } else if (showUpdate) {
    buttons.push(
      <span
        key="uptodate"
        className={hstack({
          gap: "1.5",
          px: size === "sm" ? "3.5" : "4",
          py: size === "sm" ? "2" : "2.5",
          borderRadius: "full",
          bg: "mint.300",
          color: "teal.600",
          fontWeight: "extrabold",
          fontSize: "sm",
          whiteSpace: "nowrap",
        })}
      >
        <CheckCircle2 size={16} /> Up to date
      </span>,
    );
  }

  return (
    <>
      <div className={hstack({ gap: "2", flexWrap: "wrap" })}>{buttons}</div>
      <ConfirmDialog
        open={confirmStop}
        onOpenChange={setConfirmStop}
        title={`Stop ${name}?`}
        description={`${name} will shut down and stop responding until you start it again. Nothing is deleted — you can start it back up any time.`}
        confirmLabel="Yes, stop it"
        tone="danger"
        onConfirm={handlers.onStop}
      />
      <ConfirmDialog
        open={confirmUpdate}
        onOpenChange={setConfirmUpdate}
        title={`Update ${name}?`}
        description={
          <>
            This installs the newest version. {name} will restart and may be offline for
            a minute. Nothing is deleted, and Veery automatically undoes the update if the
            new version fails to start.
            {updateImage && (
              <span
                className={css({
                  display: "block",
                  mt: "3",
                  fontSize: "sm",
                  fontFamily: "monospace",
                  color: "text",
                  wordBreak: "break-all",
                })}
              >
                {updateImage}
              </span>
            )}
          </>
        }
        confirmLabel="Update now"
        cancelLabel="Not now"
        onConfirm={handlers.onUpdate}
      />
    </>
  );
}
