import { AlertTriangle, Boxes, Plus } from "lucide-react";
import { css } from "styled-system/css";
import { flex, hstack, vstack, grid } from "styled-system/patterns";

export function Dashboard() {
  return (
    <div className={vstack({ gap: "8", alignItems: "stretch" })}>
      <div className={hstack({ justify: "space-between", flexWrap: "wrap", gap: "4" })}>
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
        <button
          className={hstack({
            gap: "2",
            px: "5",
            py: "2.5",
            borderRadius: "full",
            bg: "accent",
            color: "white",
            fontWeight: "bold",
            cursor: "pointer",
            boxShadow: "card",
            transition: "background 0.15s ease",
            _hover: { bg: "accentHover" },
          })}
        >
          <Plus size={18} />
          Add a service
        </button>
      </div>

      <section
        className={vstack({
          gap: "3",
          alignItems: "stretch",
          p: "5",
          borderRadius: "xl",
          bgGradient: "to-r",
          gradientFrom: "coral.300",
          gradientTo: "sunshine.300",
          boxShadow: "card",
        })}
      >
        <div className={hstack({ gap: "2.5" })}>
          <AlertTriangle size={20} className={css({ color: "coral.600" })} />
          <h2 className={css({ fontSize: "lg", fontWeight: "extrabold", color: "ink.900" })}>
            Needs attention
          </h2>
        </div>
        <p className={css({ color: "ink.800", fontWeight: "medium" })}>
          All clear for now. Anything that needs a look will show up here.
        </p>
      </section>

      <section className={vstack({ gap: "4", alignItems: "stretch" })}>
        <div className={hstack({ gap: "2.5" })}>
          <Boxes size={20} className={css({ color: "grape.500" })} />
          <h2 className={css({ fontSize: "lg", fontWeight: "extrabold" })}>Services</h2>
        </div>
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
                className={css({ mt: "auto", w: "20", h: "6", borderRadius: "full", bg: "mint.300" })}
              />
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
