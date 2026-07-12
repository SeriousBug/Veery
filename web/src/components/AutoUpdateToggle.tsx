import { useState } from "react";
import { Switch } from "@ark-ui/react";
import { RefreshCw } from "lucide-react";
import { css } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";
import { setAutoUpdate } from "../lib/actions";

export function AutoUpdateToggle({
  containerId,
  autoUpdate,
}: {
  containerId: string;
  autoUpdate: boolean;
}) {
  const [checked, setChecked] = useState(autoUpdate);
  const [pending, setPending] = useState(false);

  async function onChange(next: boolean) {
    setChecked(next);
    setPending(true);
    const ok = await setAutoUpdate(containerId, next);
    if (!ok) setChecked(!next);
    setPending(false);
  }

  return (
    <Switch.Root
      checked={checked}
      disabled={pending}
      onCheckedChange={(d) => onChange(d.checked)}
      className={hstack({
        justify: "space-between",
        gap: "4",
        p: "4",
        borderRadius: "lg",
        bg: "grape.50",
        borderWidth: "1px",
        borderColor: "grape.100",
        cursor: "pointer",
        _disabled: { opacity: 0.7, cursor: "not-allowed" },
      })}
    >
      <span className={hstack({ gap: "3" })}>
        <RefreshCw size={18} className={css({ color: "grape.500" })} />
        <span className={vstack({ gap: "0", alignItems: "flex-start" })}>
          <Switch.Label
            className={css({ fontWeight: "extrabold", fontSize: "sm", color: "text" })}
          >
            Keep this up to date automatically
          </Switch.Label>
          <span className={css({ fontSize: "xs", color: "textMuted" })}>
            Veery installs new versions for you when they're ready.
          </span>
        </span>
      </span>
      <Switch.Control
        className={css({
          w: "12",
          h: "7",
          borderRadius: "full",
          bg: "ink.200",
          p: "1",
          transition: "background 0.2s ease",
          flexShrink: 0,
          "&[data-state='checked']": { bg: "grape.500" },
        })}
      >
        <Switch.Thumb
          className={css({
            display: "block",
            w: "5",
            h: "5",
            borderRadius: "full",
            bg: "white",
            boxShadow: "card",
            transition: "transform 0.2s ease",
            "&[data-state='checked']": { transform: "translateX(20px)" },
          })}
        />
      </Switch.Control>
      <Switch.HiddenInput />
    </Switch.Root>
  );
}
