import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Check,
  Copy,
  Loader2,
  Mail,
  Plus,
  ShieldCheck,
  UserRound,
  Users,
} from "lucide-react";
import { css } from "styled-system/css";
import { flex, hstack, vstack, grid } from "styled-system/patterns";
import { http } from "../api/http";
import type { Invite, User } from "../api/generated";

function useCopy() {
  const [copied, setCopied] = useState<string | null>(null);
  return {
    copied,
    copy: async (text: string, key: string) => {
      try {
        await navigator.clipboard.writeText(text);
        setCopied(key);
        setTimeout(() => setCopied((c) => (c === key ? null : c)), 1800);
      } catch {
        // clipboard unavailable; ignore
      }
    },
  };
}

function CopyButton({
  text,
  copyKey,
  copied,
  onCopy,
}: {
  text: string;
  copyKey: string;
  copied: string | null;
  onCopy: (text: string, key: string) => void;
}) {
  const isCopied = copied === copyKey;
  return (
    <button
      onClick={() => onCopy(text, copyKey)}
      className={hstack({
        gap: "1.5",
        px: "3",
        py: "2",
        borderRadius: "full",
        bg: isCopied ? "mint.300" : "grape.100",
        color: isCopied ? "ink.900" : "grape.700",
        fontSize: "sm",
        fontWeight: "bold",
        cursor: "pointer",
        flexShrink: 0,
        transition: "all 0.15s ease",
        _hover: { bg: isCopied ? "mint.300" : "grape.200" },
      })}
    >
      {isCopied ? <Check size={16} /> : <Copy size={16} />}
      {isCopied ? "Copied!" : "Copy link"}
    </button>
  );
}

export function Invites() {
  const queryClient = useQueryClient();
  const { copied, copy } = useCopy();
  const [makeAdmin, setMakeAdmin] = useState(false);

  const invitesQuery = useQuery({
    queryKey: ["invites"],
    queryFn: () => http.get<Invite[]>("/api/invites"),
  });
  const usersQuery = useQuery({
    queryKey: ["users"],
    queryFn: () => http.get<User[]>("/api/users"),
  });

  const createInvite = useMutation({
    mutationFn: (isAdmin: boolean) =>
      http.post<Invite>("/api/invites", { isAdmin }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["invites"] });
    },
  });

  const invites = invitesQuery.data ?? [];
  const users = usersQuery.data ?? [];
  const fresh = createInvite.data;

  return (
    <div className={vstack({ gap: "8", alignItems: "stretch" })}>
      <div>
        <h1
          className={css({
            fontSize: "3xl",
            fontWeight: "extrabold",
            letterSpacing: "-0.02em",
          })}
        >
          Invites
        </h1>
        <p className={css({ color: "textMuted", mt: "1" })}>
          Send someone a link and they'll set up their own passkey.
        </p>
      </div>

      <section
        className={vstack({
          gap: "4",
          alignItems: "stretch",
          p: "5",
          borderRadius: "xl",
          bg: "surface",
          borderWidth: "1px",
          borderColor: "border",
          boxShadow: "card",
        })}
      >
        <div className={hstack({ gap: "2.5" })}>
          <Plus size={20} className={css({ color: "grape.500" })} />
          <h2 className={css({ fontSize: "lg", fontWeight: "extrabold" })}>Create an invite</h2>
        </div>

        <label
          className={hstack({
            gap: "3",
            cursor: "pointer",
            userSelect: "none",
          })}
        >
          <input
            type="checkbox"
            checked={makeAdmin}
            onChange={(e) => setMakeAdmin(e.target.checked)}
            className={css({ w: "5", h: "5", accentColor: "accent", cursor: "pointer" })}
          />
          <span className={css({ fontWeight: "bold" })}>Make this person an admin</span>
          <span className={css({ color: "textMuted", fontSize: "sm" })}>
            Admins can invite others and manage everything.
          </span>
        </label>

        <button
          onClick={() => createInvite.mutate(makeAdmin)}
          disabled={createInvite.isPending}
          className={hstack({
            gap: "2",
            alignSelf: "flex-start",
            px: "5",
            py: "2.5",
            borderRadius: "full",
            bg: "accent",
            color: "white",
            fontWeight: "bold",
            cursor: "pointer",
            boxShadow: "card",
            transition: "background 0.15s ease",
            _hover: { bg: "accentHover" },
            _disabled: { opacity: 0.6, cursor: "not-allowed" },
          })}
        >
          {createInvite.isPending ? (
            <Loader2 size={18} className={css({ animation: "spin 0.9s linear infinite" })} />
          ) : (
            <Plus size={18} />
          )}
          Create invite
        </button>

        {createInvite.isError && (
          <p className={css({ color: "attention", fontWeight: "bold", fontSize: "sm" })}>
            Couldn't create the invite. Please try again.
          </p>
        )}

        {fresh && (
          <div
            className={vstack({
              gap: "3",
              alignItems: "stretch",
              p: "4",
              borderRadius: "lg",
              bgGradient: "to-r",
              gradientFrom: "mint.300",
              gradientTo: "sunshine.300",
            })}
          >
            <span className={css({ fontWeight: "extrabold", color: "ink.900" })}>
              Your invite link is ready! Share it now.
            </span>
            <div className={hstack({ gap: "3", flexWrap: "wrap" })}>
              <code
                className={css({
                  flex: "1",
                  minW: "0",
                  px: "3",
                  py: "2",
                  borderRadius: "md",
                  bg: "surface",
                  fontSize: "sm",
                  fontWeight: "bold",
                  wordBreak: "break-all",
                })}
              >
                {fresh.url}
              </code>
              <CopyButton
                text={fresh.url}
                copyKey="fresh"
                copied={copied}
                onCopy={copy}
              />
            </div>
          </div>
        )}
      </section>

      <section className={vstack({ gap: "4", alignItems: "stretch" })}>
        <div className={hstack({ gap: "2.5" })}>
          <Mail size={20} className={css({ color: "grape.500" })} />
          <h2 className={css({ fontSize: "lg", fontWeight: "extrabold" })}>Pending invites</h2>
        </div>
        {invitesQuery.isLoading ? (
          <Spinner />
        ) : invites.length === 0 ? (
          <EmptyNote>No pending invites. Create one above to get started.</EmptyNote>
        ) : (
          <div className={vstack({ gap: "3", alignItems: "stretch" })}>
            {invites.map((invite) => (
              <div
                key={invite.token}
                className={flex({
                  align: "center",
                  gap: "3",
                  flexWrap: "wrap",
                  p: "4",
                  borderRadius: "lg",
                  bg: "surface",
                  borderWidth: "1px",
                  borderColor: "border",
                  boxShadow: "card",
                })}
              >
                <span
                  className={hstack({
                    gap: "1.5",
                    px: "3",
                    py: "1",
                    borderRadius: "full",
                    bg: invite.isAdmin ? "grape.100" : "ink.100",
                    color: invite.isAdmin ? "grape.700" : "textMuted",
                    fontSize: "sm",
                    fontWeight: "bold",
                    flexShrink: 0,
                  })}
                >
                  {invite.isAdmin ? <ShieldCheck size={15} /> : <UserRound size={15} />}
                  {invite.isAdmin ? "Admin" : "Member"}
                </span>
                <code
                  className={css({
                    flex: "1",
                    minW: "40",
                    fontSize: "sm",
                    color: "textMuted",
                    wordBreak: "break-all",
                  })}
                >
                  {invite.url}
                </code>
                <span className={css({ fontSize: "xs", color: "textMuted", flexShrink: 0 })}>
                  Expires {formatDate(invite.expiresAt)}
                </span>
                <CopyButton
                  text={invite.url}
                  copyKey={invite.token}
                  copied={copied}
                  onCopy={copy}
                />
              </div>
            ))}
          </div>
        )}
      </section>

      <section className={vstack({ gap: "4", alignItems: "stretch" })}>
        <div className={hstack({ gap: "2.5" })}>
          <Users size={20} className={css({ color: "teal.500" })} />
          <h2 className={css({ fontSize: "lg", fontWeight: "extrabold" })}>People</h2>
        </div>
        {usersQuery.isLoading ? (
          <Spinner />
        ) : users.length === 0 ? (
          <EmptyNote>No one has enrolled yet.</EmptyNote>
        ) : (
          <div className={grid({ columns: { base: 1, sm: 2, lg: 3 }, gap: "3" })}>
            {users.map((user) => (
              <div
                key={user.id}
                className={hstack({
                  gap: "3",
                  p: "4",
                  borderRadius: "lg",
                  bg: "surface",
                  borderWidth: "1px",
                  borderColor: "border",
                  boxShadow: "card",
                })}
              >
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
                    flexShrink: 0,
                    fontWeight: "extrabold",
                  })}
                >
                  {user.name.charAt(0).toUpperCase()}
                </span>
                <div className={vstack({ gap: "0", alignItems: "flex-start", minW: "0" })}>
                  <span className={css({ fontWeight: "bold", truncate: true, maxW: "full" })}>
                    {user.name}
                  </span>
                  {user.isAdmin && (
                    <span
                      className={css({
                        fontSize: "xs",
                        fontWeight: "bold",
                        color: "grape.600",
                      })}
                    >
                      Admin
                    </span>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function Spinner() {
  return (
    <div className={flex({ justify: "center", py: "6" })}>
      <Loader2 size={28} className={css({ color: "accent", animation: "spin 0.9s linear infinite" })} />
    </div>
  );
}

function EmptyNote({ children }: { children: React.ReactNode }) {
  return (
    <p className={css({ color: "textMuted", fontWeight: "medium" })}>{children}</p>
  );
}

function formatDate(seconds: number): string {
  return new Date(seconds * 1000).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
