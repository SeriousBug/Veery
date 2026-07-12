import { Link } from "@tanstack/react-router";
import { ChevronRight, Sparkles, Layers } from "lucide-react";
import { css } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";
import { StatusPill } from "./StatusPill";
import { ActionBar } from "./ActionBar";
import { stackAction, containerAction } from "../lib/actions";
import type { JobProgress, Stack } from "../api/generated";

export function stackBusy(
  stack: Stack,
  jobs: Map<string, JobProgress>,
): JobProgress | undefined {
  const own = jobs.get(stack.id);
  if (own) return own;
  for (const c of stack.containers) {
    const j = jobs.get(c.id);
    if (j) return j;
  }
  return undefined;
}

export function ServiceCard({
  stack,
  jobs,
}: {
  stack: Stack;
  jobs: Map<string, JobProgress>;
}) {
  const updateAvailable = stack.containers.some((c) => c.updateAvailable);
  const busy = stackBusy(stack, jobs);
  const count = stack.containers.length;

  return (
    <div
      className={vstack({
        gap: "4",
        alignItems: "stretch",
        p: "5",
        borderRadius: "lg",
        bg: "surface",
        borderWidth: "1px",
        borderColor: "border",
        boxShadow: "card",
        transition: "transform 0.15s ease, box-shadow 0.15s ease",
        _hover: { boxShadow: "pop", transform: "translateY(-2px)" },
      })}
    >
      <div className={hstack({ justify: "space-between", gap: "3", alignItems: "flex-start" })}>
        <Link
          to="/service/$id"
          params={{ id: stack.id }}
          className={hstack({
            gap: "2",
            textDecoration: "none",
            color: "text",
            minW: "0",
            _hover: { color: "grape.600" },
          })}
        >
          <span
            className={css({
              fontSize: "lg",
              fontWeight: "extrabold",
              letterSpacing: "-0.01em",
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            })}
          >
            {stack.name}
          </span>
          <ChevronRight size={18} className={css({ color: "textMuted", flexShrink: 0 })} />
        </Link>
        <StatusPill status={stack.status} size="sm" />
      </div>

      <div className={hstack({ gap: "3", color: "textMuted", fontSize: "sm", fontWeight: "medium" })}>
        <span className={hstack({ gap: "1.5" })}>
          <Layers size={15} />
          {count === 1 ? "1 service" : `${count} services`}
        </span>
        {updateAvailable && (
          <span
            className={hstack({
              gap: "1.5",
              px: "2.5",
              py: "1",
              borderRadius: "full",
              bg: "sunshine.300",
              color: "ink.900",
              fontWeight: "extrabold",
              fontSize: "xs",
            })}
          >
            <Sparkles size={13} />
            Update available
          </span>
        )}
      </div>

      <ActionBar
        name={stack.name}
        status={stack.status}
        busy={busy}
        updateAvailable={updateAvailable}
        size="sm"
        handlers={{
          onStart: () => stackAction(stack.id, "start"),
          onStop: () => stackAction(stack.id, "stop"),
          onRestart: () => stackAction(stack.id, "restart"),
          onBringUp: () => stackAction(stack.id, "bringup"),
          onUpdate: () => {
            for (const c of stack.containers) {
              if (c.updateAvailable) void containerAction(c.id, "update");
            }
          },
        }}
      />
    </div>
  );
}
