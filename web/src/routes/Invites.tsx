import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Check,
  Copy,
  KeyRound,
  LifeBuoy,
  Loader2,
  Mail,
  Plus,
  ShieldCheck,
  Trash2,
  UserRound,
  Users,
} from "lucide-react";
import { css } from "styled-system/css";
import { flex, hstack, vstack, grid } from "styled-system/patterns";
import { http, HttpError } from "../api/http";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { useAuth } from "../auth/AuthProvider";
import { toaster } from "../lib/toaster";
import type { Invite, User } from "../api/generated";

type ConfirmTarget =
  | { kind: "invite"; token: string }
  | { kind: "user"; id: string; name: string };

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
  const { user: me } = useAuth();
  const [makeAdmin, setMakeAdmin] = useState(false);
  const [confirm, setConfirm] = useState<ConfirmTarget | null>(null);
  const [recovery, setRecovery] = useState<Invite | null>(null);

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

  const revokeInvite = useMutation({
    mutationFn: (token: string) => http.del(`/api/invites/${token}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["invites"] });
      toaster.create({ type: "success", title: "Invite revoked", duration: 3000 });
    },
    onError: (err) =>
      toaster.create({
        type: "error",
        title: err instanceof HttpError ? err.message : "Couldn't revoke that invite",
        duration: 4000,
      }),
  });

  const deleteUser = useMutation({
    mutationFn: (id: string) => http.del(`/api/users/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["users"] });
      toaster.create({ type: "success", title: "Person removed", duration: 3000 });
    },
    onError: (err) =>
      toaster.create({
        type: "error",
        title: err instanceof HttpError ? err.message : "Couldn't remove that person",
        duration: 4000,
      }),
  });

  const resetUser = useMutation({
    mutationFn: (id: string) => http.post<Invite>(`/api/users/${id}/reset`),
    onSuccess: (invite) => {
      setRecovery(invite);
      queryClient.invalidateQueries({ queryKey: ["invites"] });
    },
    onError: (err) =>
      toaster.create({
        type: "error",
        title: err instanceof HttpError ? err.message : "Couldn't create a recovery link",
        duration: 4000,
      }),
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
                {invite.forUserName && (
                  <span
                    className={hstack({
                      gap: "1.5",
                      px: "3",
                      py: "1",
                      borderRadius: "full",
                      bg: "teal.100",
                      color: "teal.700",
                      fontSize: "sm",
                      fontWeight: "bold",
                      flexShrink: 0,
                    })}
                  >
                    <LifeBuoy size={15} />
                    Recovery · {invite.forUserName}
                  </span>
                )}
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
                <button
                  onClick={() => setConfirm({ kind: "invite", token: invite.token })}
                  aria-label="Revoke invite"
                  title="Revoke invite"
                  className={flex({
                    align: "center",
                    justify: "center",
                    w: "9",
                    h: "9",
                    borderRadius: "full",
                    bg: "coral.100",
                    color: "coral.600",
                    cursor: "pointer",
                    flexShrink: 0,
                    transition: "all 0.15s ease",
                    _hover: { bg: "coral.200" },
                  })}
                >
                  <Trash2 size={16} />
                </button>
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

        {recovery && (
          <div
            className={vstack({
              gap: "3",
              alignItems: "stretch",
              p: "4",
              borderRadius: "lg",
              bgGradient: "to-r",
              gradientFrom: "grape.100",
              gradientTo: "teal.100",
            })}
          >
            <div className={hstack({ gap: "2", flexWrap: "wrap" })}>
              <LifeBuoy size={18} className={css({ color: "grape.600" })} />
              <span className={css({ fontWeight: "extrabold", color: "ink.900" })}>
                Recovery link for {recovery.forUserName ?? "this person"} is ready.
              </span>
              <span className={css({ color: "textMuted", fontSize: "sm" })}>
                They open it to add a new passkey. Single use, expires {formatDate(recovery.expiresAt)}.
              </span>
            </div>
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
                {recovery.url}
              </code>
              <CopyButton
                text={recovery.url}
                copyKey="recovery"
                copied={copied}
                onCopy={copy}
              />
              <button
                onClick={() => setRecovery(null)}
                className={css({
                  px: "4",
                  py: "2",
                  borderRadius: "full",
                  bg: "ink.100",
                  color: "text",
                  fontSize: "sm",
                  fontWeight: "bold",
                  cursor: "pointer",
                  flexShrink: 0,
                  _hover: { bg: "ink.200" },
                })}
              >
                Done
              </button>
            </div>
          </div>
        )}

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
                    {me?.id === user.id && (
                      <span className={css({ color: "textMuted", fontWeight: "medium" })}> (you)</span>
                    )}
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
                {me?.id !== user.id && (
                  <div className={hstack({ gap: "2", ml: "auto", flexShrink: 0 })}>
                    <button
                      onClick={() => resetUser.mutate(user.id)}
                      disabled={resetUser.isPending && resetUser.variables === user.id}
                      aria-label={`Reset access for ${user.name}`}
                      title="Reset access — mint a recovery link"
                      className={flex({
                        align: "center",
                        justify: "center",
                        w: "9",
                        h: "9",
                        borderRadius: "full",
                        bg: "grape.100",
                        color: "grape.700",
                        cursor: "pointer",
                        transition: "all 0.15s ease",
                        _hover: { bg: "grape.200" },
                        _disabled: { opacity: 0.6, cursor: "not-allowed" },
                      })}
                    >
                      {resetUser.isPending && resetUser.variables === user.id ? (
                        <Loader2 size={16} className={css({ animation: "spin 0.9s linear infinite" })} />
                      ) : (
                        <KeyRound size={16} />
                      )}
                    </button>
                    <button
                      onClick={() =>
                        setConfirm({ kind: "user", id: user.id, name: user.name })
                      }
                      aria-label={`Remove ${user.name}`}
                      title="Remove person"
                      className={flex({
                        align: "center",
                        justify: "center",
                        w: "9",
                        h: "9",
                        borderRadius: "full",
                        bg: "coral.100",
                        color: "coral.600",
                        cursor: "pointer",
                        transition: "all 0.15s ease",
                        _hover: { bg: "coral.200" },
                      })}
                    >
                      <Trash2 size={16} />
                    </button>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </section>

      <ConfirmDialog
        open={confirm !== null}
        onOpenChange={(open) => {
          if (!open) setConfirm(null);
        }}
        title={confirm?.kind === "user" ? "Remove this person?" : "Revoke this invite?"}
        description={
          confirm?.kind === "user" ? (
            <>
              <strong>{confirm.name}</strong> will lose access immediately and all their
              passkeys will be removed. This can't be undone.
            </>
          ) : (
            "This enrollment link will stop working right away. You can always create a new one."
          )
        }
        confirmLabel={confirm?.kind === "user" ? "Remove" : "Revoke"}
        tone="danger"
        onConfirm={() => {
          if (confirm?.kind === "invite") revokeInvite.mutate(confirm.token);
          else if (confirm?.kind === "user") deleteUser.mutate(confirm.id);
        }}
      />
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
