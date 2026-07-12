import { Cpu, MemoryStick, HardDrive, Download, Upload, Activity } from "lucide-react";
import { css, cx } from "styled-system/css";
import { grid, hstack, vstack } from "styled-system/patterns";
import { Gauge } from "./Gauge";
import { useLiveData } from "../live/LiveData";
import { formatPercent, formatRate, formatUsage, ratioPct, rateLevel } from "../lib/format";
import type { RateLevel } from "../lib/format";
import type { DiskActivity } from "../api/generated";

const rateColorClass: Record<RateLevel, string> = {
  idle: css({ color: "textMuted" }),
  low: css({ color: "teal.600" }),
  mid: css({ color: "sunshine.500" }),
  high: css({ color: "coral.500" }),
};

export function HostResources() {
  const { metrics } = useLiveData();
  const host = metrics?.host;
  const hasStorage = !!host && (host.disks.length > 0 || host.diskActivity.length > 0);

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
        <Activity size={20} className={css({ color: "teal.500" })} />
        <h2 className={css({ fontSize: "lg", fontWeight: "extrabold" })}>This machine</h2>
      </div>

      {!host ? (
        <p className={css({ color: "textMuted" })}>Reading live resource usage…</p>
      ) : (
        <div className={vstack({ gap: "5", alignItems: "stretch" })}>
          <div className={grid({ columns: { base: 1, md: 2 }, gap: "5" })}>
            <Gauge
              label="Processor"
              value={formatPercent(host.cpuPercent)}
              pct={host.cpuPercent}
              icon={<Cpu size={15} />}
            />
            <Gauge
              label="Memory"
              value={formatUsage(host.memUsed, host.memTotal)}
              pct={ratioPct(host.memUsed, host.memTotal)}
              icon={<MemoryStick size={15} />}
            />
            {host.disks.map((d) => (
              <Gauge
                key={d.key}
                label={<DiskLabel mount={d.mount} />}
                value={formatUsage(d.used, d.total)}
                pct={ratioPct(d.used, d.total)}
                icon={<HardDrive size={15} />}
              />
            ))}
          </div>

          {host.diskActivity.length > 0 && (
            <div
              className={css({
                display: "flex",
                flexWrap: "wrap",
                gap: "6",
                pt: "4",
                borderTopWidth: "1px",
                borderColor: "border",
              })}
            >
              {host.diskActivity.map((a) => (
                <DiskActivityRow key={a.key} activity={a} single={host.diskActivity.length === 1} />
              ))}
            </div>
          )}
        </div>
      )}

      {host && !hasStorage && (
        <p className={css({ fontSize: "sm", color: "textMuted" })}>
          No disks to show. Pick some in Settings.
        </p>
      )}
    </section>
  );
}

function diskName(mount: string): string {
  if (mount === "/") return "Main disk";
  const segment = mount.split("/").filter(Boolean).pop();
  if (!segment) return "Disk";
  return segment.charAt(0).toUpperCase() + segment.slice(1) + " disk";
}

function DiskLabel({ mount }: { mount: string }) {
  return (
    <span className={hstack({ gap: "1.5" })}>
      {diskName(mount)}
      {mount !== "/" && (
        <span className={css({ color: "textMuted", fontWeight: "medium", opacity: 0.7 })}>
          {mount}
        </span>
      )}
    </span>
  );
}

function DiskActivityRow({ activity, single }: { activity: DiskActivity; single: boolean }) {
  const title = activity.label || (single ? "" : activity.device);
  return (
    <div className={vstack({ gap: "2", alignItems: "flex-start", minW: "0" })}>
      {title && (
        <span className={hstack({ gap: "1.5", fontSize: "sm", fontWeight: "extrabold", color: "text" })}>
          <HardDrive size={15} className={css({ color: "textMuted" })} />
          {title}
        </span>
      )}
      <div className={hstack({ gap: "6", flexWrap: "wrap" })}>
        <Bandwidth
          icon={<Download size={16} className={css({ color: "teal.500" })} />}
          label="Reading"
          value={formatRate(activity.readBytesPerSec)}
          level={rateLevel(activity.readBytesPerSec, activity.readPeakBytesPerSec)}
        />
        <Bandwidth
          icon={<Upload size={16} className={css({ color: "grape.500" })} />}
          label="Writing"
          value={formatRate(activity.writeBytesPerSec)}
          level={rateLevel(activity.writeBytesPerSec, activity.writePeakBytesPerSec)}
        />
      </div>
    </div>
  );
}

function Bandwidth({
  icon,
  label,
  value,
  level,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  level: RateLevel;
}) {
  return (
    <div className={vstack({ gap: "0.5", alignItems: "flex-start" })}>
      <span className={hstack({ gap: "1.5", fontSize: "sm", fontWeight: "bold", color: "textMuted" })}>
        {icon}
        {label}
      </span>
      <span
        className={cx(
          css({ fontSize: "xl", fontWeight: "extrabold", transition: "color 0.2s ease" }),
          rateColorClass[level],
        )}
      >
        {value}
      </span>
    </div>
  );
}
