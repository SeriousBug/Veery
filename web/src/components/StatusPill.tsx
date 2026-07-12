import { css } from "styled-system/css";
import { hstack } from "styled-system/patterns";
import { statusMeta } from "../lib/status";
import type { ContainerStatus } from "../api/generated";

export function StatusPill({
  status,
  size = "md",
}: {
  status: ContainerStatus;
  size?: "sm" | "md";
}) {
  const meta = statusMeta(status);
  const Icon = meta.icon;
  const spinning = meta.tone === "busy";
  return (
    <span
      className={hstack({
        gap: "1.5",
        px: size === "sm" ? "2.5" : "3",
        py: size === "sm" ? "1" : "1.5",
        borderRadius: "full",
        bg: meta.soft,
        color: meta.strong,
        fontWeight: "extrabold",
        fontSize: size === "sm" ? "xs" : "sm",
        whiteSpace: "nowrap",
      })}
    >
      <Icon
        size={size === "sm" ? 13 : 15}
        className={spinning ? css({ animation: "spin 1.4s linear infinite" }) : undefined}
      />
      {meta.label}
    </span>
  );
}

/** Bare colored dot, for compact rows. */
export function StatusDot({ status }: { status: ContainerStatus }) {
  const meta = statusMeta(status);
  return (
    <span
      className={css({
        w: "2.5",
        h: "2.5",
        borderRadius: "full",
        bg: meta.dot,
        flexShrink: 0,
        boxShadow: meta.tone === "ok" ? "0 0 0 4px token(colors.mint.300)" : undefined,
      })}
    />
  );
}
