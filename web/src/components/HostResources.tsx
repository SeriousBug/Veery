import { Cpu, MemoryStick, HardDrive, Download, Upload, Activity } from "lucide-react";
import { css, cx } from "styled-system/css";
import { grid, hstack, vstack } from "styled-system/patterns";
import { Gauge } from "./Gauge";
import { useLiveData } from "../live/LiveData";
import { formatPercent, formatRate, formatUsage, ratioPct, rateLevel } from "../lib/format";
import type { RateLevel } from "../lib/format";

const rateColorClass: Record<RateLevel, string> = {
  idle: css({ color: "textMuted" }),
  low: css({ color: "teal.600" }),
  mid: css({ color: "sunshine.500" }),
  high: css({ color: "coral.500" }),
};

export function HostResources() {
  const { metrics } = useLiveData();
  const host = metrics?.host;

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
              key={d.mount}
              label={<DiskLabel mount={d.mount} />}
              value={formatUsage(d.used, d.total)}
              pct={ratioPct(d.used, d.total)}
              icon={<HardDrive size={15} />}
            />
          ))}
          <div className={hstack({ gap: "6", alignSelf: "center", flexWrap: "wrap" })}>
            <Bandwidth
              icon={<Download size={16} className={css({ color: "teal.500" })} />}
              label="Reading"
              value={formatRate(host.diskReadBytesPerSec)}
              level={rateLevel(host.diskReadBytesPerSec, host.diskReadPeakBytesPerSec)}
            />
            <Bandwidth
              icon={<Upload size={16} className={css({ color: "grape.500" })} />}
              label="Writing"
              value={formatRate(host.diskWriteBytesPerSec)}
              level={rateLevel(host.diskWriteBytesPerSec, host.diskWritePeakBytesPerSec)}
            />
          </div>
        </div>
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
