import { Boxes } from "lucide-react";
import { css } from "styled-system/css";
import { flex, grid, hstack, vstack } from "styled-system/patterns";
import { useLiveData } from "../live/LiveData";
import { HostResources } from "../components/HostResources";
import { AttentionBand } from "../components/AttentionBand";
import { ServiceCard } from "../components/ServiceCard";
import { DiscoveredStacks } from "../components/DiscoveredStacks";

export function Dashboard() {
  const { stacks, jobs, loading } = useLiveData();
  const managed = stacks.filter((s) => s.managed);

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

      {loading ? (
        <SkeletonGrid />
      ) : (
        <>
          <AttentionBand stacks={stacks} jobs={jobs} />

          <section className={vstack({ gap: "4", alignItems: "stretch" })}>
            <div className={hstack({ gap: "2.5" })}>
              <Boxes size={20} className={css({ color: "grape.500" })} />
              <h2 className={css({ fontSize: "lg", fontWeight: "extrabold" })}>Services</h2>
            </div>
            {managed.length === 0 ? (
              <p className={css({ color: "textMuted" })}>
                No managed services yet. Adopt one below to get started.
              </p>
            ) : (
              <div className={grid({ columns: { base: 1, sm: 2, lg: 3 }, gap: "4" })}>
                {managed.map((stack) => (
                  <ServiceCard key={stack.id} stack={stack} jobs={jobs} />
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

function SkeletonGrid() {
  return (
    <div className={grid({ columns: { base: 1, sm: 2, lg: 3 }, gap: "4" })}>
      {[0, 1, 2].map((i) => (
        <div
          key={i}
          className={flex({
            direction: "column",
            gap: "2",
            p: "5",
            h: "36",
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
