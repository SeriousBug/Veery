import type { ReactNode } from "react";
import { Navigate } from "@tanstack/react-router";
import { Loader2 } from "lucide-react";
import { css } from "styled-system/css";
import { flex } from "styled-system/patterns";
import { useAuth } from "./AuthProvider";

function FullScreenSpinner() {
  return (
    <div className={flex({ align: "center", justify: "center", minH: "100vh", bg: "bg" })}>
      <Loader2
        size={40}
        className={css({
          color: "accent",
          animation: "spin 0.9s linear infinite",
        })}
      />
    </div>
  );
}

export function RequireAuth({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth();

  if (loading) return <FullScreenSpinner />;
  if (!user) return <Navigate to="/login" />;
  return <>{children}</>;
}
