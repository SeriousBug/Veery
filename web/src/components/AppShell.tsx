import { useState, type ReactNode } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  LayoutDashboard,
  Settings as SettingsIcon,
  Mail,
  Boxes,
  LogOut,
  Loader2,
} from "lucide-react";
import { css } from "styled-system/css";
import { flex, hstack } from "styled-system/patterns";
import { useAuth } from "../auth/AuthProvider";
import { LiveDataProvider } from "../live/LiveData";
import { MobileNav } from "./MobileNav";
import { ToasterView } from "./ToasterView";
import { ConnectionLight } from "./ConnectionLight";

const baseNavItems = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard, adminOnly: false },
  { to: "/invites", label: "Invites", icon: Mail, adminOnly: true },
  { to: "/settings", label: "Settings", icon: SettingsIcon, adminOnly: false },
] as const;

export function AppShell({ children }: { children: ReactNode }) {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const [loggingOut, setLoggingOut] = useState(false);

  const navItems = baseNavItems.filter((item) => !item.adminOnly || user?.isAdmin);

  async function handleLogout() {
    setLoggingOut(true);
    try {
      await logout();
      await navigate({ to: "/login" });
    } finally {
      setLoggingOut(false);
    }
  }

  return (
    <LiveDataProvider>
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
        <div className={hstack({ gap: "2.5" })}>
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
          <ConnectionLight />
        </div>

        <MobileNav
          navItems={navItems}
          user={user}
          loggingOut={loggingOut}
          onLogout={handleLogout}
        />

        <div className={hstack({ gap: "2", display: { base: "none", md: "flex" } })}>
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

          {user && (
            <div
              className={hstack({
                gap: "2",
                pl: "2",
                ml: "1",
                borderLeftWidth: "1px",
                borderColor: "border",
              })}
            >
              <span
                className={hstack({
                  gap: "2",
                  px: "1",
                })}
              >
                <span
                  className={flex({
                    align: "center",
                    justify: "center",
                    w: "8",
                    h: "8",
                    borderRadius: "full",
                    bgGradient: "to-br",
                    gradientFrom: "grape.400",
                    gradientTo: "teal.400",
                    color: "white",
                    fontSize: "sm",
                    fontWeight: "extrabold",
                    flexShrink: 0,
                  })}
                >
                  {user.name.charAt(0).toUpperCase()}
                </span>
                <span
                  className={css({
                    fontSize: "sm",
                    fontWeight: "bold",
                    color: "text",
                  })}
                >
                  {user.name}
                </span>
              </span>
              <button
                onClick={handleLogout}
                disabled={loggingOut}
                title="Log out"
                aria-label="Log out"
                className={flex({
                  align: "center",
                  justify: "center",
                  gap: "2",
                  px: "3",
                  py: "2",
                  borderRadius: "full",
                  fontSize: "sm",
                  fontWeight: "bold",
                  color: "textMuted",
                  cursor: "pointer",
                  transition: "all 0.15s ease",
                  _hover: { bg: "coral.300", color: "ink.900" },
                  _disabled: { opacity: 0.6, cursor: "not-allowed" },
                })}
              >
                {loggingOut ? (
                  <Loader2 size={17} className={css({ animation: "spin 0.9s linear infinite" })} />
                ) : (
                  <LogOut size={17} />
                )}
              </button>
            </div>
          )}
        </div>
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
      <ToasterView />
    </div>
    </LiveDataProvider>
  );
}
