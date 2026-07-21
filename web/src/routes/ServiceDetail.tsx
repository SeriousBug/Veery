import { Link } from "@tanstack/react-router";
import { ArrowLeft, Box, ChevronDown, Cpu, MemoryStick, Sparkles } from "lucide-react";
import { css } from "styled-system/css";
import { grid, hstack, vstack } from "styled-system/patterns";
import { useLiveData } from "../live/LiveData";
import { StatusPill } from "../components/StatusPill";
import { Gauge } from "../components/Gauge";
import { ActionBar } from "../components/ActionBar";
import { AutoUpdateToggle } from "../components/AutoUpdateToggle";
import { stackBusy } from "../components/ServiceCard";
import {
  stackAction,
  containerAction,
  forgetContainer,
  forgetStack,
  checkStackUpdate,
  checkContainerUpdate,
} from "../lib/actions";
import { formatBytes, formatPercent, formatUsage, ratioPct, formatAge } from "../lib/format";
import type { Container, ContainerMetrics, JobProgress } from "../api/generated";

export function ServiceDetail({ id }: { id: string }) {
  const { stacks, metrics, jobs, loading } = useLiveData();
  const stack = stacks.find((s) => s.id === id);

  if (!stack) {
    return (
      <div className={vstack({ gap: "4", alignItems: "flex-start" })}>
        <BackLink />
        <p className={css({ color: "textMuted" })}>
          {loading ? "Loading this service…" : "We couldn't find that service. It may have been removed."}
        </p>
      </div>
    );
  }

  const updateAvailable = stack.containers.some((c) => c.updateAvailable);
  const updateImage = (stack.containers.find((c) => c.updateAvailable) ?? stack.containers[0])
    ?.image;
  const busy = stackBusy(stack, jobs);
  const metricsById = new Map<string, ContainerMetrics>(
    (metrics?.containers ?? []).map((m) => [m.id, m]),
  );

  return (
    <div className={vstack({ gap: "6", alignItems: "stretch" })}>
      <BackLink />

      <div
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
        <div className={hstack({ justify: "space-between", gap: "3", flexWrap: "wrap" })}>
          <h1
            className={css({ fontSize: "3xl", fontWeight: "extrabold", letterSpacing: "-0.02em" })}
          >
            {stack.name}
          </h1>
          <StatusPill status={stack.status} />
        </div>
        {updateAvailable && (
          <span
            className={hstack({
              gap: "1.5",
              alignSelf: "flex-start",
              px: "3",
              py: "1.5",
              borderRadius: "full",
              bg: "sunshine.300",
              color: "ink.900",
              fontWeight: "extrabold",
              fontSize: "sm",
            })}
          >
            <Sparkles size={15} /> An update is ready to install
          </span>
        )}
        <ActionBar
          name={stack.name}
          status={stack.status}
          busy={busy}
          updateAvailable={updateAvailable}
          showUpdate={stack.managed}
          updateImage={updateImage}
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
            onCheckUpdate: () => checkStackUpdate(stack.id),
          }}
        />
      </div>

      <h2 className={css({ fontSize: "lg", fontWeight: "extrabold" })}>
        {stack.containers.length === 1 ? "Details" : "What's inside"}
      </h2>
      <div className={vstack({ gap: "4", alignItems: "stretch" })}>
        {stack.containers.map((c) => (
          <ContainerPanel
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

function BackLink() {
  return (
    <Link
      to="/"
      className={hstack({
        gap: "1.5",
        color: "textMuted",
        fontWeight: "bold",
        fontSize: "sm",
        textDecoration: "none",
        _hover: { color: "text" },
      })}
    >
      <ArrowLeft size={16} /> All services
    </Link>
  );
}

function ContainerPanel({
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
      <div className={hstack({ justify: "space-between", gap: "3", flexWrap: "wrap" })}>
        <span className={hstack({ gap: "2.5", minW: "0" })}>
          <Box size={18} className={css({ color: "grape.500" })} />
          <span className={css({ fontWeight: "extrabold", fontSize: "md" })}>{container.name}</span>
        </span>
        <StatusPill status={container.status} size="sm" />
      </div>

      <div className={grid({ columns: { base: 1, sm: 2 }, gap: "5" })}>
        <Gauge
          label="Processor"
          value={metrics ? formatPercent(metrics.cpuPercent) : "—"}
          pct={metrics?.cpuPercent ?? 0}
          icon={<Cpu size={15} />}
        />
        <Gauge label="Memory" value={memValue} pct={memPct} icon={<MemoryStick size={15} />} />
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
          // A container that no longer exists cannot be started, only built
          // again from its snapshot, which is what bringing the stack up does.
          onBringUp: () => stackAction(stackId, "bringup"),
          onUpdate: () => containerAction(container.containerName, "update"),
          onForget: () => forgetContainer(container.containerName, container.name),
          onCheckUpdate: () => checkContainerUpdate(container.containerName),
        }}
      />

      <AutoUpdateToggle containerId={container.containerName} autoUpdate={container.autoUpdate} />

      <details className={css({ "& > summary": { listStyle: "none" } })}>
        <summary
          className={hstack({
            gap: "1.5",
            cursor: "pointer",
            color: "textMuted",
            fontWeight: "bold",
            fontSize: "sm",
            _hover: { color: "text" },
            "& svg": { transition: "transform 0.2s ease" },
          })}
        >
          <ChevronDown size={15} />
          Technical details
        </summary>
        <dl
          className={grid({
            columns: { base: 1, sm: 2 },
            gap: "3",
            mt: "3",
            fontSize: "sm",
          })}
        >
          <Detail label="Image" value={container.image} />
          <Detail label="Container name" value={container.containerName} />
          <Detail label="Docker state" value={container.state || "unknown"} />
          <Detail label="Health" value={container.health || "n/a"} />
          <Detail label="Restarts" value={String(container.restartCount)} />
          <Detail label="Created" value={formatAge(container.createdAt)} />
          <Detail label="Container ID" value={container.id} />
        </dl>
      </details>
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div className={vstack({ gap: "0.5", alignItems: "flex-start", minW: "0" })}>
      <dt className={css({ color: "textMuted", fontWeight: "bold" })}>{label}</dt>
      <dd
        className={css({
          color: "text",
          fontWeight: "medium",
          fontFamily: "monospace",
          wordBreak: "break-all",
        })}
      >
        {value}
      </dd>
    </div>
  );
}
