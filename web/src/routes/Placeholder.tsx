import type { ReactNode } from "react";
import { css } from "styled-system/css";
import { vstack } from "styled-system/patterns";

export function Placeholder({
  title,
  children,
}: {
  title: string;
  children?: ReactNode;
}) {
  return (
    <div className={vstack({ gap: "3", alignItems: "flex-start" })}>
      <h1
        className={css({
          fontSize: "3xl",
          fontWeight: "extrabold",
          letterSpacing: "-0.02em",
        })}
      >
        {title}
      </h1>
      <div className={css({ color: "textMuted" })}>
        {children ?? "Coming soon."}
      </div>
    </div>
  );
}
