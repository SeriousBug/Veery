import { AlertTriangle, LifeBuoy, PartyPopper, Loader2 } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { css } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";
import { StatusPill } from "./StatusPill";
import { stackBusy } from "./ServiceCard";
import { stackAction } from "../lib/actions";
import { needsAttention, attentionReason, stackNeedsBringUp } from "../lib/status";
import type { JobProgress, Stack } from "../api/generated";

export function AttentionBand({
  stacks,
  jobs,
}: {
  stacks: Stack[];
  jobs: Map<string, JobProgress>;
}) {
  const problems = stacks.filter((s) => needsAttention(s.status));

  if (problems.length === 0) {
    return (
      <section
        className={hstack({
          gap: "3",
          p: "5",
          borderRadius: "xl",
          bgGradient: "to-r",
          gradientFrom: "mint.300",
          gradientTo: "teal.300",
          boxShadow: "card",
        })}
      >
        <PartyPopper size={22} className={css({ color: "teal.600" })} />
        <div>
          <h2 className={css({ fontSize: "lg", fontWeight: "extrabold", color: "ink.900" })}>
            All good!
          </h2>
          <p className={css({ color: "ink.800", fontWeight: "medium" })}>
            Every service is running happily. Nothing needs your attention.
          </p>
        </div>
      </section>
    );
  }

  return (
    <section
      className={vstack({
        gap: "3",
        alignItems: "stretch",
        p: "5",
        borderRadius: "xl",
        bgGradient: "to-r",
        gradientFrom: "coral.300",
        gradientTo: "sunshine.300",
        boxShadow: "card",
      })}
    >
      <div className={hstack({ gap: "2.5" })}>
        <AlertTriangle size={20} className={css({ color: "coral.600" })} />
        <h2 className={css({ fontSize: "lg", fontWeight: "extrabold", color: "ink.900" })}>
          Needs attention
        </h2>
      </div>

      <div className={vstack({ gap: "2.5", alignItems: "stretch" })}>
        {problems.map((stack) => {
          const busy = stackBusy(stack, jobs);
          // A stack with any missing container aggregates to needs_attention,
          // but Restart can't recreate a removed container — bringup can.
          const bringUp = stackNeedsBringUp(stack);
          return (
            <div
              key={stack.id}
              className={hstack({
                justify: "space-between",
                gap: "3",
                flexWrap: "wrap",
                p: "3.5",
                borderRadius: "lg",
                bg: "surface",
                boxShadow: "card",
              })}
            >
              <div className={vstack({ gap: "1", alignItems: "flex-start", minW: "0" })}>
                <div className={hstack({ gap: "3", minW: "0" })}>
                  <Link
                    to="/service/$id"
                    params={{ id: stack.id }}
                    className={css({
                      fontWeight: "extrabold",
                      fontSize: "md",
                      color: "text",
                      textDecoration: "none",
                      _hover: { color: "grape.600" },
                    })}
                  >
                    {stack.name}
                  </Link>
                  <StatusPill status={stack.status} size="sm" />
                </div>
                <p className={css({ fontSize: "sm", color: "ink.700", fontWeight: "medium" })}>
                  {attentionReason(stack)}
                </p>
              </div>

              {busy ? (
                <span
                  className={hstack({
                    gap: "2",
                    px: "4",
                    py: "2.5",
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
              ) : (
                <button
                  onClick={() => stackAction(stack.id, bringUp ? "bringup" : "restart")}
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
                  })}
                >
                  <LifeBuoy size={16} />
                  Get it running again
                </button>
              )}
            </div>
          );
        })}
      </div>
    </section>
  );
}
