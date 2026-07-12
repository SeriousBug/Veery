import type { ReactNode } from "react";
import { css } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";
import { clampPct } from "../lib/format";

const fillBase = {
  h: "full",
  borderRadius: "full",
  bgGradient: "to-r",
  transition: "width 0.6s cubic-bezier(0.22, 1, 0.36, 1)",
} as const;

const CALM = css({ ...fillBase, gradientFrom: "teal.400", gradientTo: "grape.400" });
const WARM = css({ ...fillBase, gradientFrom: "sunshine.400", gradientTo: "coral.400" });
const HOT = css({ ...fillBase, gradientFrom: "coral.400", gradientTo: "coral.600" });

function fillClass(pct: number): string {
  if (pct >= 90) return HOT;
  if (pct >= 75) return WARM;
  return CALM;
}

export function Gauge({
  label,
  value,
  pct,
  icon,
}: {
  label: ReactNode;
  value: string;
  pct: number;
  icon?: ReactNode;
}) {
  const filled = clampPct(pct);
  return (
    <div className={vstack({ gap: "2", alignItems: "stretch" })}>
      <div className={hstack({ justify: "space-between", gap: "2" })}>
        <span
          className={hstack({
            gap: "1.5",
            fontSize: "sm",
            fontWeight: "bold",
            color: "textMuted",
          })}
        >
          {icon}
          {label}
        </span>
        <span className={css({ fontSize: "sm", fontWeight: "extrabold", color: "text" })}>
          {value}
        </span>
      </div>
      <div
        className={css({ h: "2.5", borderRadius: "full", bg: "ink.100", overflow: "hidden" })}
      >
        <div className={fillClass(filled)} style={{ width: `${filled}%` }} />
      </div>
    </div>
  );
}
