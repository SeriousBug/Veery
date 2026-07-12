import { Dialog, Portal } from "@ark-ui/react";
import type { ReactNode } from "react";
import { css } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";

export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  cancelLabel = "Never mind",
  tone = "accent",
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: ReactNode;
  confirmLabel: string;
  cancelLabel?: string;
  tone?: "accent" | "danger";
  onConfirm: () => void;
}) {
  const confirmBg = tone === "danger" ? "coral.500" : "accent";
  const confirmHover = tone === "danger" ? "coral.600" : "accentHover";
  return (
    <Dialog.Root
      open={open}
      onOpenChange={(d) => onOpenChange(d.open)}
      lazyMount
      unmountOnExit
    >
      <Portal>
        <Dialog.Backdrop
          className={css({
            position: "fixed",
            inset: "0",
            bg: "rgba(23, 19, 32, 0.45)",
            backdropFilter: "blur(2px)",
            zIndex: "50",
          })}
        />
        <Dialog.Positioner
          className={css({
            position: "fixed",
            inset: "0",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            p: "4",
            zIndex: "50",
          })}
        >
          <Dialog.Content
            className={vstack({
              gap: "4",
              alignItems: "stretch",
              w: "full",
              maxW: "sm",
              p: "6",
              bg: "surface",
              borderRadius: "xl",
              boxShadow: "pop",
            })}
          >
            <Dialog.Title
              className={css({ fontSize: "xl", fontWeight: "extrabold", color: "text" })}
            >
              {title}
            </Dialog.Title>
            <Dialog.Description className={css({ color: "textMuted", lineHeight: "1.5" })}>
              {description}
            </Dialog.Description>
            <div className={hstack({ gap: "3", justify: "flex-end", mt: "1" })}>
              <Dialog.CloseTrigger
                className={css({
                  px: "4",
                  py: "2.5",
                  borderRadius: "full",
                  fontWeight: "bold",
                  color: "text",
                  bg: "ink.100",
                  cursor: "pointer",
                  _hover: { bg: "ink.200" },
                })}
              >
                {cancelLabel}
              </Dialog.CloseTrigger>
              <button
                onClick={() => {
                  onConfirm();
                  onOpenChange(false);
                }}
                className={css({
                  px: "5",
                  py: "2.5",
                  borderRadius: "full",
                  fontWeight: "extrabold",
                  color: "white",
                  bg: confirmBg,
                  cursor: "pointer",
                  boxShadow: "card",
                  _hover: { bg: confirmHover },
                })}
              >
                {confirmLabel}
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Positioner>
      </Portal>
    </Dialog.Root>
  );
}
