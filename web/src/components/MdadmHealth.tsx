import { useState } from "react";
import { HardDriveDownload, ShieldCheck, ShieldAlert, ShieldX, Loader } from "lucide-react";
import { useMutation } from "@tanstack/react-query";
import { css, cx } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";
import { useLiveData } from "../live/LiveData";
import { useAuth } from "../auth/AuthProvider";
import { http, HttpError } from "../api/http";
import { toaster } from "../lib/toaster";
import { clampPct, formatRate } from "../lib/format";
import { ConfirmDialog } from "./ConfirmDialog";
import type { MdArray, MdArrayState } from "../api/generated";

const stateColor: Record<MdArrayState, string> = {
  healthy: css({ color: "teal.600" }),
  recovering: css({ color: "sunshine.500" }),
  degraded: css({ color: "coral.500" }),
  failed: css({ color: "coral.600" }),
};

const stateLabel: Record<MdArrayState, string> = {
  healthy: "Healthy",
  recovering: "Working",
  degraded: "Degraded",
  failed: "Failed",
};

function StateIcon({ state }: { state: MdArrayState }) {
  const cls = stateColor[state];
  switch (state) {
    case "healthy":
      return <ShieldCheck size={16} className={cls} />;
    case "recovering":
      return <Loader size={16} className={cx(cls, css({ animation: "spin 2s linear infinite" }))} />;
    case "degraded":
      return <ShieldAlert size={16} className={cls} />;
    case "failed":
      return <ShieldX size={16} className={cls} />;
  }
}

const actionLabel: Record<string, string> = {
  check: "Scrubbing",
  resync: "Resyncing",
  recover: "Recovering",
  reshape: "Reshaping",
};

export function MdadmHealth() {
  const { metrics } = useLiveData();
  const arrays = metrics?.host.mdadm;

  if (!arrays || arrays.length === 0) return null;

  return (
    <section
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
      <div className={hstack({ gap: "2.5" })}>
        <HardDriveDownload size={20} className={css({ color: "grape.500" })} />
        <h2 className={css({ fontSize: "lg", fontWeight: "extrabold" })}>RAID health</h2>
      </div>

      <div className={vstack({ gap: "5", alignItems: "stretch" })}>
        {arrays.map((a) => (
          <ArrayRow key={a.name} array={a} />
        ))}
      </div>
    </section>
  );
}

function ArrayRow({ array }: { array: MdArray }) {
  const { user } = useAuth();
  const [confirm, setConfirm] = useState(false);
  const syncing = array.syncAction !== "idle";

  const startScan = useMutation({
    mutationFn: () => http.post(`/api/mdadm/${array.name}/scan`),
    onSuccess: () =>
      toaster.create({ type: "success", title: "Scan started", duration: 3000 }),
    onError: (err) =>
      toaster.create({
        type: "error",
        title: "Couldn't start scan",
        description: err instanceof HttpError ? err.message : "Please try again.",
      }),
  });

  return (
    <div className={vstack({ gap: "3", alignItems: "stretch" })}>
      <div className={hstack({ justify: "space-between", gap: "3", flexWrap: "wrap" })}>
        <div className={hstack({ gap: "2.5" })}>
          <StateIcon state={array.state} />
          <span className={css({ fontWeight: "extrabold", color: "text" })}>{array.name}</span>
          <span className={css({ fontSize: "sm", color: "textMuted", fontWeight: "medium" })}>
            {array.level}
          </span>
          <span className={cx(css({ fontSize: "sm", fontWeight: "bold" }), stateColor[array.state])}>
            {stateLabel[array.state]}
          </span>
        </div>

        <div className={hstack({ gap: "3" })}>
          <Members array={array} />
          {user?.isAdmin && (
            <button
              onClick={() => setConfirm(true)}
              disabled={syncing || startScan.isPending}
              className={css({
                px: "3.5",
                py: "1.5",
                borderRadius: "full",
                fontSize: "sm",
                fontWeight: "bold",
                color: "text",
                bg: "ink.100",
                cursor: "pointer",
                _hover: { bg: "ink.200" },
                _disabled: { opacity: 0.5, cursor: "not-allowed" },
              })}
            >
              Start scan
            </button>
          )}
        </div>
      </div>

      {syncing && <SyncProgress array={array} />}

      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        title={`Scan ${array.name}?`}
        description="A data scrub reads every block to catch errors. It runs in the background and can drive disk I/O for a long time, but does not change your data."
        confirmLabel="Start scan"
        onConfirm={() => startScan.mutate()}
      />
    </div>
  );
}

function Members({ array }: { array: MdArray }) {
  return (
    <span className={hstack({ gap: "2" })} title={`${array.devicesUp} of ${array.devicesTotal} up`}>
      <span className={hstack({ gap: "1" })}>
        {array.members.map((m) => (
          <span
            key={m.device}
            title={`${m.device} ${m.up ? "up" : "down"}`}
            className={css({
              w: "2.5",
              h: "2.5",
              borderRadius: "full",
              bg: m.up ? "teal.500" : "coral.500",
            })}
          />
        ))}
      </span>
      <span className={css({ fontSize: "sm", color: "textMuted", fontWeight: "bold" })}>
        {array.devicesUp}/{array.devicesTotal}
      </span>
    </span>
  );
}

function SyncProgress({ array }: { array: MdArray }) {
  const pct = clampPct(array.syncPercent);
  const label = actionLabel[array.syncAction] ?? array.syncAction;
  return (
    <div className={vstack({ gap: "1.5", alignItems: "stretch" })}>
      <div className={hstack({ justify: "space-between", gap: "2", fontSize: "sm" })}>
        <span className={css({ fontWeight: "bold", color: "textMuted" })}>
          {label} — {pct.toFixed(1)}%
        </span>
        <span className={css({ color: "textMuted" })}>
          {formatRate(array.syncSpeedKBs * 1024)}
          {array.syncFinish && ` · ${array.syncFinish} left`}
        </span>
      </div>
      <div className={css({ h: "2.5", borderRadius: "full", bg: "ink.100", overflow: "hidden" })}>
        <div
          className={css({
            h: "full",
            borderRadius: "full",
            bgGradient: "to-r",
            gradientFrom: "sunshine.400",
            gradientTo: "grape.400",
            transition: "width 0.6s cubic-bezier(0.22, 1, 0.36, 1)",
          })}
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  );
}
