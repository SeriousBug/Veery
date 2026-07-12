import { useState } from "react";
import { Dialog, Portal } from "@ark-ui/react";
import { Link, type LinkProps } from "@tanstack/react-router";
import { Menu, X, LogOut, Loader2, type LucideIcon } from "lucide-react";
import { css } from "styled-system/css";
import { flex, hstack, vstack } from "styled-system/patterns";
import type { User } from "../api/generated";

export type NavItem = {
  to: LinkProps["to"];
  label: string;
  icon: LucideIcon;
};

const iconButton = css({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  w: "10",
  h: "10",
  borderRadius: "full",
  color: "text",
  cursor: "pointer",
  transition: "all 0.15s ease",
  _hover: { bg: "ink.100" },
});

export function MobileNav({
  navItems,
  user,
  loggingOut,
  onLogout,
}: {
  navItems: readonly NavItem[];
  user: User | null;
  loggingOut: boolean;
  onLogout: () => void;
}) {
  const [open, setOpen] = useState(false);

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(d) => setOpen(d.open)}
      lazyMount
      unmountOnExit
    >
      <Dialog.Trigger
        aria-label="Open menu"
        className={css({
          display: { base: "flex", md: "none" },
          alignItems: "center",
          justifyContent: "center",
          w: "10",
          h: "10",
          borderRadius: "full",
          color: "text",
          cursor: "pointer",
          transition: "all 0.15s ease",
          _hover: { bg: "ink.100" },
        })}
      >
        <Menu size={22} />
      </Dialog.Trigger>

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
            justifyContent: "flex-end",
            zIndex: "50",
          })}
        >
          <Dialog.Content
            className={vstack({
              gap: "6",
              alignItems: "stretch",
              w: "72",
              maxW: "85vw",
              h: "full",
              p: "5",
              bg: "surface",
              boxShadow: "pop",
            })}
          >
            <div className={hstack({ justify: "space-between" })}>
              <Dialog.Title
                className={css({ fontSize: "lg", fontWeight: "extrabold", color: "text" })}
              >
                Menu
              </Dialog.Title>
              <Dialog.CloseTrigger aria-label="Close menu" className={iconButton}>
                <X size={20} />
              </Dialog.CloseTrigger>
            </div>

            {user && (
              <div className={hstack({ gap: "2.5", minW: "0" })}>
                <span
                  className={flex({
                    align: "center",
                    justify: "center",
                    w: "10",
                    h: "10",
                    borderRadius: "full",
                    bgGradient: "to-br",
                    gradientFrom: "grape.400",
                    gradientTo: "teal.400",
                    color: "white",
                    fontWeight: "extrabold",
                    flexShrink: 0,
                  })}
                >
                  {user.name.charAt(0).toUpperCase()}
                </span>
                <span
                  className={css({
                    fontWeight: "bold",
                    color: "text",
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                  })}
                >
                  {user.name}
                </span>
              </div>
            )}

            <nav className={vstack({ gap: "1", alignItems: "stretch" })}>
              {navItems.map(({ to, label, icon: Icon }) => (
                <Link
                  key={to}
                  to={to}
                  activeOptions={{ exact: to === "/" }}
                  onClick={() => setOpen(false)}
                  className={hstack({
                    gap: "3",
                    px: "4",
                    py: "3",
                    borderRadius: "md",
                    fontSize: "md",
                    fontWeight: "bold",
                    color: "textMuted",
                    textDecoration: "none",
                    _hover: { bg: "ink.100", color: "text" },
                    "&[data-status='active']": {
                      bg: "grape.100",
                      color: "grape.700",
                    },
                  })}
                >
                  <Icon size={19} />
                  {label}
                </Link>
              ))}
            </nav>

            {user && (
              <button
                onClick={onLogout}
                disabled={loggingOut}
                className={hstack({
                  gap: "3",
                  mt: "auto",
                  px: "4",
                  py: "3",
                  borderRadius: "md",
                  fontSize: "md",
                  fontWeight: "bold",
                  color: "textMuted",
                  cursor: "pointer",
                  _hover: { bg: "coral.300", color: "ink.900" },
                  _disabled: { opacity: 0.6, cursor: "not-allowed" },
                })}
              >
                {loggingOut ? (
                  <Loader2
                    size={19}
                    className={css({ animation: "spin 0.9s linear infinite" })}
                  />
                ) : (
                  <LogOut size={19} />
                )}
                Log out
              </button>
            )}
          </Dialog.Content>
        </Dialog.Positioner>
      </Portal>
    </Dialog.Root>
  );
}
