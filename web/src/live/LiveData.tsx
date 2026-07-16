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

/** Push-stream connection state, surfaced so the header can show a live light. */
export type ConnectionState = "connecting" | "open" | "closed";

interface LiveDataValue {
  stacks: Stack[];
  metrics: MetricsSnapshot | null;
  /** Latest in-flight job per target id (stack or container). */
  jobs: Map<string, JobProgress>;
  loading: boolean;
  connection: ConnectionState;
}

const LiveDataContext = createContext<LiveDataValue | null>(null);

const OFFLINE_TOAST = "ws-offline";
const OFFLINE_GRACE_MS = 2500;

const LOADING_VERB: Record<string, string> = {
  start: "Starting up",
  stop: "Stopping",
  restart: "Restarting",
  update: "Updating",
  pull: "Downloading update for",
  bringup: "Bringing back up",
  recreate: "Recreating",
  adopt: "Taking over management of",
};

function loadingTitle(job: JobProgress, name: string | null): string {
  const verb = LOADING_VERB[job.kind];
  const who = name ?? job.target;
  if (!verb) return who ? `Working on ${who}…` : "Working on it…";
  return who ? `${verb} ${who}…` : `${verb}…`;
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
  const [connection, setConnection] = useState<ConnectionState>("connecting");
  const seenToasts = useRef<Set<string>>(new Set());
  const stacksRef = useRef<Stack[]>([]);

  useEffect(() => {
    stacksRef.current = stacks;
  }, [stacks]);

  useEffect(() => {
    let cancelled = false;
    let offlineTimer = 0;

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

    // The full job picture, sent when the connection opens. It is the only way a
    // page loaded mid-update learns an update is running, and the only way a page
    // that was disconnected (because Veery was restarting itself) learns how the
    // update it was watching turned out.
    const onJobs = (msg: WSMessage) => {
      const jobs = msg.jobs ?? [];
      for (const job of jobs) handleJob(job);

      // Anything we are still showing a spinner for, that the server does not
      // know about, is finished and gone. Without this it spins forever.
      const known = new Set(jobs.map((j) => j.id));
      for (const id of seenToasts.current) {
        if (!known.has(id)) toaster.remove(id);
      }
      seenToasts.current = new Set(known);
    };

    const unsubs = [
      wsClient.subscribe("stacks", onStacks),
      wsClient.subscribe("metrics", onMetrics),
      wsClient.subscribe("job", onJob),
      wsClient.subscribe("jobs", onJobs),
      wsClient.onStatus(handleStatus),
    ];
    wsClient.connect();

    // Veery restarts its own container to update itself, so losing the stream is
    // an expected part of an update rather than an error. Say that, instead of
    // leaving the page looking live but frozen. Held back briefly so an ordinary
    // blip doesn't flash a scary message.
    function handleStatus(status: "open" | "closed") {
      setConnection(status);
      if (status === "open") {
        window.clearTimeout(offlineTimer);
        toaster.remove(OFFLINE_TOAST);
        return;
      }
      window.clearTimeout(offlineTimer);
      offlineTimer = window.setTimeout(() => {
        toaster.create({
          id: OFFLINE_TOAST,
          type: "loading",
          title: "Reconnecting to Veery…",
          description: "It may be restarting to finish an update.",
          duration: Number.POSITIVE_INFINITY,
        });
      }, OFFLINE_GRACE_MS);
    }

    function handleJob(job: JobProgress) {
      setJobs((prev) => {
        const next = new Map(prev);
        if (job.done) next.delete(job.target);
        else next.set(job.target, job);
        return next;
      });

      const name = targetName(stacksRef.current, job.target);
      const description = job.message || undefined;
      if (!job.done) {
        const opts = { title: loadingTitle(job, name), description, type: "loading" };
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
      window.clearTimeout(offlineTimer);
      for (const off of unsubs) off();
      wsClient.close();
    };
  }, []);

  const value: LiveDataValue = { stacks, metrics, jobs, loading, connection };
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
