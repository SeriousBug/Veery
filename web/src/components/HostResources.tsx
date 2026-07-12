import { Cpu, MemoryStick, HardDrive, Download, Upload, Activity } from "lucide-react";
import { css } from "styled-system/css";
import { grid, hstack, vstack } from "styled-system/patterns";
import { Gauge } from "./Gauge";
import { useLiveData } from "../live/LiveData";
import { formatPercent, formatRate, formatUsage, ratioPct } from "../lib/format";

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
              label={`Storage · ${d.mount}`}
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
            />
            <Bandwidth
              icon={<Upload size={16} className={css({ color: "grape.500" })} />}
              label="Writing"
              value={formatRate(host.diskWriteBytesPerSec)}
            />
          </div>
        </div>
      )}
    </section>
  );
}

function Bandwidth({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
}) {
  return (
    <div className={vstack({ gap: "0.5", alignItems: "flex-start" })}>
      <span className={hstack({ gap: "1.5", fontSize: "sm", fontWeight: "bold", color: "textMuted" })}>
        {icon}
        {label}
      </span>
      <span className={css({ fontSize: "xl", fontWeight: "extrabold", color: "text" })}>
        {value}
      </span>
    </div>
  );
}
