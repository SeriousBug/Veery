import { Switch } from "@ark-ui/react";
import { css } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";

export function ToggleField({
  title,
  hint,
  checked,
  disabled,
  onChange,
}: {
  title: string;
  hint: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <Switch.Root
      checked={checked}
      disabled={disabled}
      onCheckedChange={(d) => onChange(d.checked)}
      className={hstack({
        justify: "space-between",
        gap: "4",
        cursor: "pointer",
        _disabled: { opacity: 0.7, cursor: "not-allowed" },
      })}
    >
      <span className={vstack({ gap: "0.5", alignItems: "flex-start" })}>
        <Switch.Label className={css({ fontWeight: "extrabold", fontSize: "md", color: "text" })}>
          {title}
        </Switch.Label>
        <span className={css({ fontSize: "sm", color: "textMuted" })}>{hint}</span>
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
