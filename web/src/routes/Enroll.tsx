import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { startRegistration } from "@simplewebauthn/browser";
import type { PublicKeyCredentialCreationOptionsJSON } from "@simplewebauthn/browser";
import { Loader2, Sparkles, Wand2 } from "lucide-react";
import { css } from "styled-system/css";
import { vstack } from "styled-system/patterns";
import { http, HttpError } from "../api/http";
import { useAuth } from "../auth/AuthProvider";
import { AuthCard, ErrorBanner } from "./Login";

interface CreationOptions {
  publicKey: PublicKeyCredentialCreationOptionsJSON;
}

export function Enroll({ token }: { token: string }) {
  const navigate = useNavigate();
  const { refresh } = useAuth();
  const [name, setName] = useState("My passkey");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!token) {
    return (
      <AuthCard>
        <Header title="Hmm, this link looks incomplete">
          This enrollment link is missing its invite code. Ask your admin to send you a fresh one.
        </Header>
      </AuthCard>
    );
  }

  async function createPasskey() {
    setBusy(true);
    setError(null);
    try {
      const options = await http.post<CreationOptions>("/auth/register/begin", {
        token,
        name: name.trim() || "My passkey",
      });
      const credential = await startRegistration({ optionsJSON: options.publicKey });
      await http.post("/auth/register/finish", credential);
      await refresh();
      await navigate({ to: "/" });
    } catch (err) {
      if (err instanceof DOMException && err.name === "NotAllowedError") {
        setError("That was cancelled. Give it another try when you're ready.");
      } else if (err instanceof HttpError && err.status >= 400 && err.status < 500) {
        setError("This invite link is invalid or has expired. Ask your admin for a new one.");
      } else {
        setError("Something went wrong setting up your passkey. Please try again.");
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthCard>
      <span
        className={css({
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          w: "20",
          h: "20",
          borderRadius: "full",
          bgGradient: "to-br",
          gradientFrom: "sunshine.400",
          gradientTo: "coral.400",
          color: "white",
          boxShadow: "pop",
        })}
      >
        <Sparkles size={40} strokeWidth={2.2} />
      </span>

      <Header title="Let's set you up!">
        Pick a name for this device, then create a passkey. It's your key to sign in from now on.
      </Header>

      <label className={vstack({ gap: "1.5", alignItems: "stretch", w: "full" })}>
        <span className={css({ fontSize: "sm", fontWeight: "bold", color: "text" })}>
          Name this passkey
        </span>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="My passkey"
          disabled={busy}
          className={css({
            w: "full",
            px: "4",
            py: "3",
            borderRadius: "md",
            borderWidth: "1px",
            borderColor: "border",
            bg: "bg",
            fontSize: "md",
            fontWeight: "medium",
            _focus: { outline: "none", borderColor: "accent" },
          })}
        />
      </label>

      <button
        onClick={createPasskey}
        disabled={busy}
        className={css({
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
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
          <Wand2 size={22} />
        )}
        {busy ? "Creating your passkey…" : "Create your passkey"}
      </button>

      {error && <ErrorBanner>{error}</ErrorBanner>}
    </AuthCard>
  );
}

function Header({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className={vstack({ gap: "1.5", textAlign: "center" })}>
      <h1
        className={css({
          fontSize: "3xl",
          fontWeight: "extrabold",
          letterSpacing: "-0.02em",
        })}
      >
        {title}
      </h1>
      <p className={css({ color: "textMuted", fontWeight: "medium" })}>{children}</p>
    </div>
  );
}
