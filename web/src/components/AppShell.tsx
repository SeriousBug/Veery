import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import {
  LayoutDashboard,
  Settings as SettingsIcon,
  Mail,
  Boxes,
} from "lucide-react";
import { css } from "styled-system/css";
import { flex, hstack } from "styled-system/patterns";

const navItems = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard },
  { to: "/invites", label: "Invites", icon: Mail },
  { to: "/settings", label: "Settings", icon: SettingsIcon },
] as const;

export function AppShell({ children }: { children: ReactNode }) {
  return (
    <div
      className={flex({
        direction: "column",
        minH: "100vh",
        bg: "bg",
      })}
    >
      <header
        className={hstack({
          justify: "space-between",
          px: { base: "4", md: "8" },
          h: "16",
          bg: "surface",
          borderBottomWidth: "1px",
          borderColor: "border",
          boxShadow: "card",
          position: "sticky",
          top: "0",
          zIndex: "10",
        })}
      >
        <Link to="/" className={hstack({ gap: "2.5", textDecoration: "none" })}>
          <span
            className={flex({
              align: "center",
              justify: "center",
              w: "10",
              h: "10",
              borderRadius: "lg",
              bgGradient: "to-br",
              gradientFrom: "grape.400",
              gradientTo: "teal.400",
              color: "white",
              boxShadow: "pop",
            })}
          >
            <Boxes size={22} strokeWidth={2.4} />
          </span>
          <span
            className={css({
              fontSize: "xl",
              fontWeight: "extrabold",
              letterSpacing: "-0.02em",
              color: "text",
            })}
          >
            Veery
          </span>
        </Link>

        <nav className={hstack({ gap: "1" })}>
          {navItems.map(({ to, label, icon: Icon }) => (
            <Link
              key={to}
              to={to}
              activeOptions={{ exact: to === "/" }}
              className={hstack({
                gap: "2",
                px: "3.5",
                py: "2",
                borderRadius: "full",
                fontSize: "sm",
                fontWeight: "bold",
                color: "textMuted",
                textDecoration: "none",
                transition: "all 0.15s ease",
                _hover: { bg: "ink.100", color: "text" },
                "&[data-status='active']": {
                  bg: "grape.100",
                  color: "grape.700",
                },
              })}
            >
              <Icon size={17} />
              {label}
            </Link>
          ))}
        </nav>
      </header>

      <main
        className={css({
          flex: "1",
          w: "full",
          maxW: "6xl",
          mx: "auto",
          px: { base: "4", md: "8" },
          py: { base: "6", md: "10" },
        })}
      >
        {children}
      </main>
    </div>
  );
}
