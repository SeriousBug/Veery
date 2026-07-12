import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { startAuthentication } from "@simplewebauthn/browser";
import type { PublicKeyCredentialRequestOptionsJSON } from "@simplewebauthn/browser";
import { Fingerprint, KeyRound, Loader2, PartyPopper } from "lucide-react";
import { css } from "styled-system/css";
import { flex, vstack } from "styled-system/patterns";
import { http, HttpError } from "../api/http";
import { useAuth } from "../auth/AuthProvider";

interface RequestOptions {
  publicKey: PublicKeyCredentialRequestOptionsJSON;
}

export function Login() {
  const navigate = useNavigate();
  const { refresh } = useAuth();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function signIn() {
    setBusy(true);
    setError(null);
    try {
      const options = await http.post<RequestOptions>("/auth/login/begin");
      const credential = await startAuthentication({
        optionsJSON: options.publicKey,
      });
      await http.post("/auth/login/finish", credential);
      await refresh();
      await navigate({ to: "/" });
    } catch (err) {
      if (err instanceof DOMException && err.name === "NotAllowedError") {
        setError("That was cancelled. Give it another try when you're ready.");
      } else if (err instanceof HttpError && err.status >= 400 && err.status < 500) {
        setError("No passkey found — ask an admin for an invite.");
      } else {
        setError("Something went wrong signing in. Please try again.");
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthCard>
      <span
        className={flex({
          align: "center",
          justify: "center",
          w: "20",
          h: "20",
          borderRadius: "full",
          bgGradient: "to-br",
          gradientFrom: "grape.400",
          gradientTo: "teal.400",
          color: "white",
          boxShadow: "pop",
        })}
      >
        <PartyPopper size={40} strokeWidth={2.2} />
      </span>

      <div className={vstack({ gap: "1.5", textAlign: "center" })}>
        <h1
          className={css({
            fontSize: "3xl",
            fontWeight: "extrabold",
            letterSpacing: "-0.02em",
          })}
        >
          Welcome back!
        </h1>
        <p className={css({ color: "textMuted", fontWeight: "medium" })}>
          Your passkey is all you need. No passwords, ever.
        </p>
      </div>

      <button
        onClick={signIn}
        disabled={busy}
        className={flex({
          align: "center",
          justify: "center",
          gap: "3",
          w: "full",
          px: "6",
          py: "4",
          borderRadius: "full",
          bg: "accent",
          color: "white",
          fontSize: "lg",
          fontWeight: "extrabold",
          cursor: "pointer",
          boxShadow: "card",
          transition: "all 0.15s ease",
          _hover: { bg: "accentHover" },
          _disabled: { opacity: 0.6, cursor: "not-allowed" },
        })}
      >
        {busy ? (
          <Loader2 size={22} className={css({ animation: "spin 0.9s linear infinite" })} />
        ) : (
          <Fingerprint size={22} />
        )}
        {busy ? "Signing you in…" : "Sign in with your passkey"}
      </button>

      {error && <ErrorBanner>{error}</ErrorBanner>}
    </AuthCard>
  );
}

export function AuthCard({ children }: { children: React.ReactNode }) {
  return (
    <div className={flex({ align: "center", justify: "center", minH: "100vh", bg: "bg", p: "4" })}>
      <div
        className={vstack({
          gap: "6",
          alignItems: "center",
          w: "full",
          maxW: "md",
          p: { base: "6", md: "10" },
          borderRadius: "xl",
          bg: "surface",
          borderWidth: "1px",
          borderColor: "border",
          boxShadow: "pop",
        })}
      >
        {children}
      </div>
    </div>
  );
}

export function ErrorBanner({ children }: { children: React.ReactNode }) {
  return (
    <div
      className={flex({
        align: "center",
        gap: "2.5",
        w: "full",
        px: "4",
        py: "3",
        borderRadius: "md",
        bg: "coral.300",
        color: "ink.900",
        fontWeight: "bold",
        fontSize: "sm",
      })}
    >
      <KeyRound size={18} className={css({ flexShrink: 0, color: "coral.600" })} />
      {children}
    </div>
  );
}
