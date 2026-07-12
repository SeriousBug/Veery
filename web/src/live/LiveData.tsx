import {
  createContext,
  use,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { http } from "../api/http";
import { wsClient } from "../api/ws";
import { toaster } from "../lib/toaster";
import type {
  JobProgress,
  MetricsSnapshot,
  Stack,
  WSMessage,
} from "../api/generated";

interface LiveDataValue {
  stacks: Stack[];
  metrics: MetricsSnapshot | null;
  /** Latest in-flight job per target id (stack or container). */
  jobs: Map<string, JobProgress>;
  loading: boolean;
}

const LiveDataContext = createContext<LiveDataValue | null>(null);

const LOADING_VERB: Record<string, string> = {
  start: "Starting up…",
  stop: "Stopping…",
  restart: "Restarting…",
  update: "Installing update…",
  pull: "Downloading update…",
  bringup: "Bringing it back up…",
  recreate: "Recreating…",
  adopt: "Taking over management…",
};

function loadingTitle(job: JobProgress): string {
  return LOADING_VERB[job.kind] ?? "Working on it…";
}

/** Resolve a friendly display name for a job target (stack or container id/name). */
function targetName(stacks: Stack[], target: string): string | null {
  for (const s of stacks) {
    if (s.id === target) return s.name;
    for (const c of s.containers) {
      if (c.id === target || c.containerName === target) return c.name;
    }
  }
  return null;
}

/** Brief, celebratory success message keyed on the action kind. */
function successToast(job: JobProgress, name: string | null): { title: string; description?: string } {
  const who = name ?? "It";
  switch (job.kind) {
    case "update":
      return { title: `${who} is updated` };
    case "restart":
    case "bringup":
    case "recreate":
    case "start":
      return { title: `${who} is back up and running` };
    case "stop":
      return { title: `${who} is stopped` };
    case "adopt":
      return { title: `${who} is now managed by Veery` };
    default:
      return { title: "All set!", description: job.message || "Done." };
  }
}

export function LiveDataProvider({ children }: { children: ReactNode }) {
  const [stacks, setStacks] = useState<Stack[]>([]);
  const [metrics, setMetrics] = useState<MetricsSnapshot | null>(null);
  const [jobs, setJobs] = useState<Map<string, JobProgress>>(new Map());
  const [loading, setLoading] = useState(true);
  const seenToasts = useRef<Set<string>>(new Set());
  const stacksRef = useRef<Stack[]>([]);

  useEffect(() => {
    stacksRef.current = stacks;
  }, [stacks]);

  useEffect(() => {
    let cancelled = false;

    http
      .get<Stack[]>("/api/stacks")
      .then((data) => {
        if (cancelled) return;
        setStacks(data ?? []);
        setLoading(false);
      })
      .catch(() => {
        // WS replay will fill state in shortly; keep the skeleton until then.
      });

    const onStacks = (msg: WSMessage) => {
      if (msg.stacks) {
        setStacks(msg.stacks);
        setLoading(false);
      }
    };
    const onMetrics = (msg: WSMessage) => {
      if (msg.metrics) setMetrics(msg.metrics);
    };
    const onJob = (msg: WSMessage) => {
      if (msg.job) handleJob(msg.job);
    };

    const unsubs = [
      wsClient.subscribe("stacks", onStacks),
      wsClient.subscribe("metrics", onMetrics),
      wsClient.subscribe("job", onJob),
    ];
    wsClient.connect();

    function handleJob(job: JobProgress) {
      setJobs((prev) => {
        const next = new Map(prev);
        if (job.done) next.delete(job.target);
        else next.set(job.target, job);
        return next;
      });

      const description = job.message || undefined;
      if (!job.done) {
        const opts = { title: loadingTitle(job), description, type: "loading" };
        if (seenToasts.current.has(job.id)) {
          toaster.update(job.id, opts);
        } else {
          seenToasts.current.add(job.id);
          toaster.create({ id: job.id, duration: Number.POSITIVE_INFINITY, ...opts });
        }
        return;
      }

      const done = job.error
        ? {
            type: "error",
            title: "Something went wrong",
            description: job.error,
            duration: 8000,
          }
        : {
            type: "success",
            ...successToast(job, targetName(stacksRef.current, job.target)),
            duration: 4000,
          };
      if (seenToasts.current.has(job.id)) {
        toaster.update(job.id, done);
      } else {
        seenToasts.current.add(job.id);
        toaster.create({ id: job.id, ...done });
      }
    }

    return () => {
      cancelled = true;
      for (const off of unsubs) off();
      wsClient.close();
    };
  }, []);

  const value: LiveDataValue = { stacks, metrics, jobs, loading };
  return <LiveDataContext value={value}>{children}</LiveDataContext>;
}

export function useLiveData(): LiveDataValue {
  const ctx = use(LiveDataContext);
  if (!ctx) throw new Error("useLiveData must be used within LiveDataProvider");
  return ctx;
}

/** Look up a stack plus its containers by stack id, for the detail route. */
export function useStack(id: string): Stack | undefined {
  const { stacks } = useLiveData();
  return stacks.find((s) => s.id === id);
}
