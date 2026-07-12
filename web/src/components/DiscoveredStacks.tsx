import { useState } from "react";
import { Compass, PlusCircle, Loader2 } from "lucide-react";
import { css } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";
import { stackAction } from "../lib/actions";
import type { Stack } from "../api/generated";

export function DiscoveredStacks({ stacks }: { stacks: Stack[] }) {
  const discovered = stacks.filter((s) => !s.managed);
  if (discovered.length === 0) return null;

  return (
    <section className={vstack({ gap: "4", alignItems: "stretch" })}>
      <div className={hstack({ gap: "2.5" })}>
        <Compass size={20} className={css({ color: "teal.500" })} />
        <h2 className={css({ fontSize: "lg", fontWeight: "extrabold" })}>Found on this machine</h2>
      </div>
      <div className={vstack({ gap: "3", alignItems: "stretch" })}>
        {discovered.map((s) => (
          <DiscoveredRow key={s.id} stack={s} />
        ))}
      </div>
    </section>
  );
}

function DiscoveredRow({ stack }: { stack: Stack }) {
  const [adopting, setAdopting] = useState(false);

  async function adopt() {
    setAdopting(true);
    const ok = await stackAction(stack.id, "adopt");
    if (!ok) setAdopting(false);
    // On success the "stacks" push will re-render this out of the discovered list.
  }

  return (
    <div
      className={hstack({
        justify: "space-between",
        gap: "3",
        flexWrap: "wrap",
        p: "4",
        borderRadius: "lg",
        bg: "surface",
        borderWidth: "1px",
        borderStyle: "dashed",
        borderColor: "grape.200",
        boxShadow: "card",
      })}
    >
      <div className={vstack({ gap: "0.5", alignItems: "flex-start", minW: "0" })}>
        <span className={css({ fontWeight: "extrabold", fontSize: "md" })}>{stack.name}</span>
        <span className={css({ fontSize: "sm", color: "textMuted" })}>
          Start managing this so Veery can restart or update it.
        </span>
      </div>
      <button
        onClick={adopt}
        disabled={adopting}
        className={hstack({
          gap: "1.5",
          px: "5",
          py: "2.5",
          borderRadius: "full",
          bg: "accent",
          color: "white",
          fontWeight: "extrabold",
          fontSize: "sm",
          cursor: "pointer",
          boxShadow: "card",
          _hover: { bg: "accentHover" },
          _disabled: { opacity: 0.6, cursor: "not-allowed" },
        })}
      >
        {adopting ? (
          <Loader2 size={16} className={css({ animation: "spin 0.9s linear infinite" })} />
        ) : (
          <PlusCircle size={16} />
        )}
        Adopt
      </button>
    </div>
  );
}
