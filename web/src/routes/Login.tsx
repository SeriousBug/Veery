import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { startAuthentication } from "@simplewebauthn/browser";
import type { PublicKeyCredentialRequestOptionsJSON } from "@simplewebauthn/browser";
import { Fingerprint, KeyRound, Loader2, PartyPopper, ShieldCheck } from "lucide-react";
import { css } from "styled-system/css";
import { flex, vstack } from "styled-system/patterns";
import { http, HttpError } from "../api/http";
import type { AuthProviders } from "../api/generated";
import { useAuth } from "../auth/AuthProvider";

interface RequestOptions {
  publicKey: PublicKeyCredentialRequestOptionsJSON;
}

// Short codes the OIDC callback puts in the URL, turned back into sentences.
const oidcErrorMessages: Record<string, string> = {
  oidc_denied: "The identity provider cancelled the sign-in.",
  oidc_state: "That sign-in expired or was started elsewhere. Please try again.",
  oidc_failed: "Could not sign you in with single sign-on. Try again, or use your passkey.",
};

export function Login({ ssoError = "" }: { ssoError?: string }) {
  const navigate = useNavigate();
  const { refresh } = useAuth();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const { data: providers } = useQuery({
    queryKey: ["auth", "providers"],
    queryFn: () => http.get<AuthProviders>("/auth/providers"),
    staleTime: Infinity,
  });
  const message =
    error ?? (ssoError ? (oidcErrorMessages[ssoError] ?? "Could not sign you in. Please try again.") : null);

  async function signIn() {
    setBusy(true);
    setError(null);
    try {
      const options = await http.post<RequestOptions>("/auth/login/begin");
      const credential = await startAuthentication({
        optionsJSON: options.publicKey,
      });
      await http.post("/auth/login/finish", credential as unknown as Record<string, unknown>);
      await refresh();
      await navigate({ to: "/" });
    } catch (err) {
      if (err instanceof DOMException && err.name === "NotAllowedError") {
        setError("That was cancelled. Give it another try when you're ready.");
      } else if (err instanceof HttpError && err.status >= 400 && err.status < 500) {
        setError("No passkey found. Ask an admin for an invite.");
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

      {providers?.oidc && (
        <>
          <div
            className={flex({ align: "center", gap: "3", w: "full", color: "textMuted", fontSize: "sm" })}
          >
            <span className={css({ flex: "1", h: "1px", bg: "border" })} />
            or
            <span className={css({ flex: "1", h: "1px", bg: "border" })} />
          </div>
          <a
            href="/auth/oidc/start"
            className={flex({
              align: "center",
              justify: "center",
              gap: "3",
              w: "full",
              px: "6",
              py: "4",
              borderRadius: "full",
              bg: "surface",
              color: "text",
              fontSize: "lg",
              fontWeight: "extrabold",
              cursor: "pointer",
              borderWidth: "1px",
              borderColor: "border",
              textDecoration: "none",
              boxShadow: "card",
              transition: "all 0.15s ease",
              _hover: { borderColor: "accent", color: "accent" },
            })}
          >
            <ShieldCheck size={22} />
            Sign in with {providers.oidcName || "SSO"}
          </a>
        </>
      )}

      {message && <ErrorBanner>{message}</ErrorBanner>}
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
