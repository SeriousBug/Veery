import { Link } from "@tanstack/react-router";
import { ChevronRight, Sparkles, Layers, Box, Cpu, MemoryStick } from "lucide-react";
import { css } from "styled-system/css";
import { grid, hstack, vstack } from "styled-system/patterns";
import { StatusPill } from "./StatusPill";
import { Gauge } from "./Gauge";
import { ActionBar } from "./ActionBar";
import { stackAction, containerAction, forgetStack, forgetContainer } from "../lib/actions";
import { formatBytes, formatPercent, formatUsage, ratioPct } from "../lib/format";
import type { Container, ContainerMetrics, JobProgress, Stack } from "../api/generated";

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
  metricsById,
}: {
  stack: Stack;
  jobs: Map<string, JobProgress>;
  metricsById: Map<string, ContainerMetrics>;
}) {
  const updateAvailable = stack.containers.some((c) => c.updateAvailable);
  const busy = stackBusy(stack, jobs);
  const count = stack.containers.length;
  const updateImage = (stack.containers.find((c) => c.updateAvailable) ?? stack.containers[0])
    ?.image;

  return (
    <div
      className={vstack({
        gap: "5",
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
          {count === 1 ? "1 container" : `${count} containers`}
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
        showUpdate={stack.managed}
        updateImage={updateImage}
        size="sm"
        handlers={{
          onStart: () => stackAction(stack.id, "start"),
          onStop: () => stackAction(stack.id, "stop"),
          onRestart: () => stackAction(stack.id, "restart"),
          onBringUp: () => stackAction(stack.id, "bringup"),
          onUpdate: () => {
            for (const c of stack.containers) {
              if (c.updateAvailable) void containerAction(c.containerName, "update");
            }
          },
          onForget: () => forgetStack(stack.id, stack.name),
        }}
      />

      <div className={vstack({ gap: "3", alignItems: "stretch" })}>
        <span
          className={css({
            fontSize: "xs",
            fontWeight: "extrabold",
            letterSpacing: "0.04em",
            textTransform: "uppercase",
            color: "textMuted",
          })}
        >
          {count === 1 ? "Container" : "Containers"}
        </span>
        {stack.containers.map((c) => (
          <ContainerRow
            key={c.id}
            container={c}
            stackId={stack.id}
            metrics={metricsById.get(c.id)}
            busy={jobs.get(c.id)}
          />
        ))}
      </div>
    </div>
  );
}

function ContainerRow({
  container,
  stackId,
  metrics,
  busy,
}: {
  container: Container;
  stackId: string;
  metrics: ContainerMetrics | undefined;
  busy: JobProgress | undefined;
}) {
  const memPct = metrics && metrics.memLimit > 0 ? ratioPct(metrics.memUsed, metrics.memLimit) : 0;
  const memValue = metrics
    ? metrics.memLimit > 0
      ? formatUsage(metrics.memUsed, metrics.memLimit)
      : formatBytes(metrics.memUsed)
    : "—";

  return (
    <div
      className={vstack({
        gap: "3",
        alignItems: "stretch",
        p: "4",
        borderRadius: "md",
        bg: "bg",
        borderWidth: "1px",
        borderColor: "border",
      })}
    >
      <div className={hstack({ justify: "space-between", gap: "3", alignItems: "flex-start" })}>
        <span className={hstack({ gap: "2", minW: "0" })}>
          <Box size={16} className={css({ color: "grape.500", flexShrink: 0 })} />
          <span className={vstack({ gap: "0.5", alignItems: "flex-start", minW: "0" })}>
            <span
              className={css({
                fontWeight: "extrabold",
                fontSize: "sm",
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
                maxW: "full",
              })}
            >
              {container.name}
            </span>
            <span
              className={css({
                fontSize: "xs",
                color: "textMuted",
                fontFamily: "monospace",
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
                maxW: "full",
              })}
            >
              {container.image}
            </span>
          </span>
        </span>
        <StatusPill status={container.status} size="sm" />
      </div>

      <div className={grid({ columns: 2, gap: "4" })}>
        <Gauge
          label="CPU"
          value={metrics ? formatPercent(metrics.cpuPercent) : "—"}
          pct={metrics?.cpuPercent ?? 0}
          icon={<Cpu size={14} />}
        />
        <Gauge label="Memory" value={memValue} pct={memPct} icon={<MemoryStick size={14} />} />
      </div>

      <ActionBar
        name={container.name}
        status={container.status}
        busy={busy}
        updateAvailable={container.updateAvailable}
        showUpdate={container.managed}
        updateImage={container.image}
        size="sm"
        handlers={{
          onStart: () => containerAction(container.id, "start"),
          onStop: () => containerAction(container.id, "stop"),
          onRestart: () => containerAction(container.id, "restart"),
          onBringUp: () => stackAction(stackId, "bringup"),
          onUpdate: () => containerAction(container.containerName, "update"),
          onForget: () => forgetContainer(container.containerName, container.name),
        }}
      />
    </div>
  );
}
