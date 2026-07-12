import { Toast, Toaster } from "@ark-ui/react";
import { CheckCircle2, AlertTriangle, Loader2, Info, X } from "lucide-react";
import { css } from "styled-system/css";
import { hstack, vstack } from "styled-system/patterns";
import { toaster } from "../lib/toaster";

const ACCENT: Record<string, string> = {
  success: "mint.500",
  error: "coral.500",
  loading: "grape.500",
  info: "grape.400",
};

function ToastIcon({ type }: { type?: string }) {
  const cls = css({ flexShrink: 0 });
  if (type === "success")
    return <CheckCircle2 size={20} className={css({ color: "mint.500", flexShrink: 0 })} />;
  if (type === "error")
    return <AlertTriangle size={20} className={css({ color: "coral.600", flexShrink: 0 })} />;
  if (type === "loading")
    return (
      <Loader2
        size={20}
        className={css({ color: "grape.500", flexShrink: 0, animation: "spin 0.9s linear infinite" })}
      />
    );
  return <Info size={20} className={cls} />;
}

export function ToasterView() {
  return (
    <Toaster toaster={toaster}>
      {(toast) => (
        <Toast.Root
          className={hstack({
            gap: "3",
            alignItems: "flex-start",
            w: { base: "calc(100vw - 32px)", sm: "sm" },
            p: "4",
            bg: "surface",
            borderRadius: "lg",
            borderWidth: "1px",
            borderColor: "border",
            borderLeftWidth: "4px",
            borderLeftColor: ACCENT[toast.type ?? "info"] ?? "grape.400",
            boxShadow: "pop",
          })}
        >
          <ToastIcon type={toast.type} />
          <div className={vstack({ gap: "0.5", alignItems: "stretch", flex: "1", minW: "0" })}>
            <Toast.Title
              className={css({ fontWeight: "extrabold", fontSize: "sm", color: "text" })}
            />
            <Toast.Description
              className={css({ fontSize: "sm", color: "textMuted", wordBreak: "break-word" })}
            />
          </div>
          <Toast.CloseTrigger
            aria-label="Dismiss"
            className={css({
              color: "textMuted",
              cursor: "pointer",
              borderRadius: "full",
              p: "0.5",
              _hover: { color: "text", bg: "ink.100" },
            })}
          >
            <X size={16} />
          </Toast.CloseTrigger>
        </Toast.Root>
      )}
    </Toaster>
  );
}
