import {
  CheckCircle2,
  PauseCircle,
  AlertTriangle,
  RefreshCw,
  CloudOff,
  HelpCircle,
  type LucideIcon,
} from "lucide-react";
import type { ContainerStatus, Stack } from "../api/generated";

export type StatusTone = "ok" | "idle" | "attention" | "busy";

export interface StatusMeta {
  label: string;
  icon: LucideIcon;
  tone: StatusTone;
  /** Panda color token for a solid dot/accent. */
  dot: string;
  /** Panda color token for a soft background. */
  soft: string;
  /** Panda color token for readable text on the soft background. */
  strong: string;
}

const MAP: Record<string, StatusMeta> = {
  running: {
    label: "Running",
    icon: CheckCircle2,
    tone: "ok",
    dot: "mint.500",
    soft: "mint.300",
    strong: "mint.500",
  },
  stopped: {
    label: "Stopped",
    icon: PauseCircle,
    tone: "idle",
    dot: "ink.400",
    soft: "ink.100",
    strong: "ink.600",
  },
  needs_attention: {
    label: "Needs attention",
    icon: AlertTriangle,
    tone: "attention",
    dot: "coral.500",
    soft: "coral.300",
    strong: "coral.600",
  },
  updating: {
    label: "Updating",
    icon: RefreshCw,
    tone: "busy",
    dot: "grape.500",
    soft: "grape.100",
    strong: "grape.700",
  },
  missing: {
    label: "Missing",
    icon: CloudOff,
    tone: "attention",
    dot: "coral.500",
    soft: "coral.300",
    strong: "coral.600",
  },
};

export function statusMeta(status: ContainerStatus): StatusMeta {
  return (
    MAP[status] ?? {
      label: status || "Unknown",
      icon: HelpCircle,
      tone: "idle",
      dot: "ink.400",
      soft: "ink.100",
      strong: "ink.600",
    }
  );
}

export function needsAttention(status: ContainerStatus): boolean {
  return status === "needs_attention" || status === "missing";
}

/**
 * True when a stack has a removed container that a plain restart can't recreate,
 * so recovery must use "bringup" instead.
 */
export function stackNeedsBringUp(stack: Stack): boolean {
  return (
    stack.status === "missing" ||
    stack.containers.some((c) => c.status === "missing")
  );
}

/**
 * Plain-language explanation of why a stack is in the attention band, picking the
 * most severe symptom across its containers. Written for non-technical users.
 */
export function attentionReason(stack: Stack): string {
  const cs = stack.containers;
  const some = (fn: (c: Stack["containers"][number]) => boolean) => cs.some(fn);

  if (stackNeedsBringUp(stack)) {
    return "One part isn't running and needs to be started back up.";
  }
  if (some((c) => c.state === "exited" || c.state === "dead")) {
    return "This stopped unexpectedly.";
  }
  if (some((c) => c.state === "restarting")) {
    return "This keeps restarting.";
  }
  if (some((c) => c.health === "unhealthy")) {
    return "This is running but not responding properly.";
  }
  return "This stopped unexpectedly.";
}
