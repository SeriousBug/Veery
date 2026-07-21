import { useState } from "react";
import { Boxes, RefreshCw, Loader2 } from "lucide-react";
import { css } from "styled-system/css";
import { flex, grid, hstack, vstack } from "styled-system/patterns";
import { useLiveData } from "../live/LiveData";
import { checkAllUpdates } from "../lib/actions";
import { HostResources } from "../components/HostResources";
import { MdadmHealth } from "../components/MdadmHealth";
import { AttentionBand } from "../components/AttentionBand";
import { ServiceCard } from "../components/ServiceCard";
import { DiscoveredStacks } from "../components/DiscoveredStacks";
import type { ContainerMetrics } from "../api/generated";

export function Dashboard() {
  const { stacks, metrics, jobs, loading } = useLiveData();
  const managed = stacks.filter((s) => s.managed);
  const metricsById = new Map<string, ContainerMetrics>(
    (metrics?.containers ?? []).map((m) => [m.id, m]),
  );

  return (
    <div className={vstack({ gap: "8", alignItems: "stretch" })}>
      <div>
        <h1
          className={css({
            fontSize: "3xl",
            fontWeight: "extrabold",
            letterSpacing: "-0.02em",
          })}
        >
          Your services
        </h1>
        <p className={css({ color: "textMuted", mt: "1" })}>
          Everything running on this machine, at a glance.
        </p>
      </div>

      <HostResources />

      <MdadmHealth />

      {loading ? (
        <SkeletonGrid />
      ) : (
        <>
          <AttentionBand stacks={stacks} jobs={jobs} />

          <section className={vstack({ gap: "4", alignItems: "stretch" })}>
            <div className={hstack({ gap: "2.5", justify: "space-between" })}>
              <div className={hstack({ gap: "2.5" })}>
                <Boxes size={20} className={css({ color: "grape.500" })} />
                <h2 className={css({ fontSize: "lg", fontWeight: "extrabold" })}>Services</h2>
              </div>
              {managed.length > 0 && <CheckAllButton />}
            </div>
            {managed.length === 0 ? (
              <p className={css({ color: "textMuted" })}>
                Nothing set up yet — add one of the services found below.
              </p>
            ) : (
              <div className={grid({ columns: { base: 1, lg: 2 }, gap: "5" })}>
                {managed.map((stack) => (
                  <ServiceCard
                    key={stack.id}
                    stack={stack}
                    jobs={jobs}
                    metricsById={metricsById}
                  />
                ))}
              </div>
            )}
          </section>

          <DiscoveredStacks stacks={stacks} />
        </>
      )}
    </div>
  );
}

function CheckAllButton() {
  const [checking, setChecking] = useState(false);
  const run = () => {
    setChecking(true);
    void checkAllUpdates().finally(() => setChecking(false));
  };
  return (
    <button
      onClick={run}
      disabled={checking}
      className={hstack({
        gap: "1.5",
        px: "3.5",
        py: "2",
        borderRadius: "full",
        bg: "ink.100",
        color: "text",
        fontWeight: "extrabold",
        fontSize: "sm",
        cursor: "pointer",
        whiteSpace: "nowrap",
        transition: "all 0.15s ease",
        _hover: { bg: "ink.200" },
        _disabled: { opacity: 0.6, cursor: "not-allowed" },
      })}
    >
      {checking ? (
        <Loader2 size={16} className={css({ animation: "spin 0.9s linear infinite" })} />
      ) : (
        <RefreshCw size={16} />
      )}
      {checking ? "Checking…" : "Check for updates"}
    </button>
  );
}

function SkeletonGrid() {
  return (
    <div className={grid({ columns: { base: 1, lg: 2 }, gap: "5" })}>
      {[0, 1].map((i) => (
        <div
          key={i}
          className={flex({
            direction: "column",
            gap: "2",
            p: "5",
            h: "72",
            borderRadius: "lg",
            bg: "surface",
            borderWidth: "1px",
            borderColor: "border",
            boxShadow: "card",
          })}
        >
          <div className={css({ w: "24", h: "4", borderRadius: "full", bg: "ink.100" })} />
          <div className={css({ w: "16", h: "3", borderRadius: "full", bg: "ink.100" })} />
          <div
            className={css({ mt: "auto", w: "20", h: "6", borderRadius: "full", bg: "ink.100" })}
          />
        </div>
      ))}
    </div>
  );
}
