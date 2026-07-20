import { useEffect, useMemo, useRef, useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Activity,
  ArrowUpCircle,
  Download,
  KeyRound,
  Loader2,
  PackageMinus,
  PackagePlus,
  ScrollText,
  Search,
} from "lucide-react";
import { css } from "styled-system/css";
import { flex, hstack, vstack } from "styled-system/patterns";
import { http } from "../api/http";
import { wsClient } from "../api/ws";
import { useLiveData } from "../live/LiveData";
import {
  EventAuth,
  EventContainerAdopted,
  EventContainerMissing,
  EventContainerStatus,
  EventUpdateApplied,
  EventUpdateAvailable,
  type Event as LogEvent,
  type EventPage,
  type NotificationEvent,
  type Stack,
  type WSMessage,
} from "../api/generated";

/** Display metadata per event type: a label, an icon, and a theme colour pair. */
const EVENT_META: Record<
  NotificationEvent,
  { label: string; icon: typeof Activity; bg: string; fg: string }
> = {
  [EventContainerStatus]: { label: "Status change", icon: Activity, bg: "teal.100", fg: "teal.700" },
  [EventContainerMissing]: { label: "Removed", icon: PackageMinus, bg: "coral.100", fg: "coral.600" },
  [EventContainerAdopted]: { label: "Adopted", icon: PackagePlus, bg: "mint.300", fg: "ink.900" },
  [EventUpdateApplied]: { label: "Update", icon: ArrowUpCircle, bg: "grape.100", fg: "grape.700" },
  [EventUpdateAvailable]: { label: "Update available", icon: Download, bg: "sunshine.300", fg: "ink.900" },
  [EventAuth]: { label: "Account", icon: KeyRound, bg: "ink.100", fg: "textMuted" },
};

const FILTERS: { value: NotificationEvent | ""; label: string }[] = [
  { value: "", label: "Everything" },
  { value: EventContainerStatus, label: "Status changes" },
  { value: EventContainerMissing, label: "Removals" },
  { value: EventContainerAdopted, label: "Adoptions" },
  { value: EventUpdateApplied, label: "Updates" },
  { value: EventUpdateAvailable, label: "Updates available" },
  { value: EventAuth, label: "Account" },
];

const PAGE_SIZE = 50;

export function Events() {
  const [filter, setFilter] = useState<NotificationEvent | "">("");
  const [search, setSearch] = useState("");
  const query = useDebounced(search, 250);
  const { stacks } = useLiveData();

  const events = useInfiniteQuery({
    queryKey: ["events", filter, query],
    initialPageParam: "",
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams({ limit: String(PAGE_SIZE) });
      if (pageParam) params.set("cursor", pageParam);
      if (filter) params.set("event", filter);
      if (query) params.set("q", query);
      return http.get<EventPage>(`/api/events?${params.toString()}`);
    },
    getNextPageParam: (last) => last.nextCursor || undefined,
  });

  // Events that arrive over the WS after the first page loaded are held here and
  // prepended, so a page the user has already scrolled past stays stable while
  // the head grows. Reset whenever the filter or search changes, since the live
  // items were matched against the old predicate.
  const [live, setLive] = useState<LogEvent[]>([]);
  useEffect(() => setLive([]), [filter, query]);

  useEffect(() => {
    const onEvent = (msg: WSMessage) => {
      const ev = msg.event;
      if (!ev) return;
      if (filter && ev.event !== filter) return;
      if (query && !matchesSearch(ev, query)) return;
      setLive((prev) => [ev, ...prev]);
    };
    const unsub = wsClient.subscribe("event", onEvent);
    return unsub;
  }, [filter, query]);

  const fetched = useMemo(
    () => events.data?.pages.flatMap((p) => p.items) ?? [],
    [events.data],
  );

  // Merge live items ahead of fetched ones, dropping any live id that a page
  // already carries so a refetch cannot double up a row.
  const items = useMemo(() => {
    const fetchedIds = new Set(fetched.map((e) => e.id));
    const merged = [...live.filter((e) => !fetchedIds.has(e.id)), ...fetched];
    const seen = new Set<number>();
    return merged.filter((e) => (seen.has(e.id) ? false : (seen.add(e.id), true)));
  }, [live, fetched]);

  const groups = useMemo(() => groupByDay(items), [items]);

  return (
    <div className={vstack({ gap: "8", alignItems: "stretch" })}>
      <div>
        <h1 className={css({ fontSize: "3xl", fontWeight: "extrabold", letterSpacing: "-0.02em" })}>
          Event log
        </h1>
        <p className={css({ color: "textMuted", mt: "1" })}>
          Everything Veery has noticed, whether or not it was sent to a notification channel.
        </p>
      </div>

      <div className={flex({ gap: "3", flexWrap: "wrap", align: "center" })}>
        <div
          className={hstack({
            gap: "2",
            flex: "1",
            minW: "56",
            px: "3.5",
            py: "2.5",
            borderRadius: "full",
            bg: "surface",
            borderWidth: "1px",
            borderColor: "border",
            boxShadow: "card",
          })}
        >
          <Search size={17} className={css({ color: "textMuted", flexShrink: 0 })} />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search events…"
            className={css({
              flex: "1",
              minW: "0",
              bg: "transparent",
              border: "none",
              outline: "none",
              fontSize: "sm",
              fontWeight: "medium",
              color: "text",
              _placeholder: { color: "textMuted" },
            })}
          />
        </div>
        <div className={hstack({ gap: "1.5", flexWrap: "wrap" })}>
          {FILTERS.map((f) => (
            <button
              key={f.value || "all"}
              onClick={() => setFilter(f.value)}
              className={css({
                px: "3.5",
                py: "2",
                borderRadius: "full",
                fontSize: "sm",
                fontWeight: "bold",
                cursor: "pointer",
                transition: "all 0.15s ease",
                bg: filter === f.value ? "grape.100" : "surface",
                color: filter === f.value ? "grape.700" : "textMuted",
                borderWidth: "1px",
                borderColor: filter === f.value ? "grape.200" : "border",
                _hover: { color: "text" },
              })}
            >
              {f.label}
            </button>
          ))}
        </div>
      </div>

      {events.isLoading ? (
        <Spinner />
      ) : items.length === 0 ? (
        <EmptyNote>
          {search || filter
            ? "Nothing matches those filters yet."
            : "No events recorded yet. They'll show up here as things happen."}
        </EmptyNote>
      ) : (
        <div
          className={vstack({
            gap: "0",
            alignItems: "stretch",
            borderRadius: "xl",
            bg: "surface",
            borderWidth: "1px",
            borderColor: "border",
            boxShadow: "card",
            overflow: "hidden",
          })}
        >
          {groups.map((group) => (
            <div key={group.day} className={vstack({ gap: "0", alignItems: "stretch" })}>
              <h2
                className={css({
                  px: "4",
                  py: "1.5",
                  fontSize: "xs",
                  fontWeight: "extrabold",
                  color: "textMuted",
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                  bg: "bg",
                  borderBottomWidth: "1px",
                  borderColor: "border",
                })}
              >
                {group.day}
              </h2>
              {group.items.map((ev) => (
                <EventRow key={ev.id} event={ev} stacks={stacks} />
              ))}
            </div>
          ))}

          {events.hasNextPage && (
            <button
              onClick={() => events.fetchNextPage()}
              disabled={events.isFetchingNextPage}
              className={hstack({
                gap: "2",
                justify: "center",
                px: "4",
                py: "3",
                bg: "bg",
                fontSize: "sm",
                fontWeight: "bold",
                color: "textMuted",
                cursor: "pointer",
                transition: "all 0.15s ease",
                _hover: { bg: "ink.100", color: "text" },
                _disabled: { opacity: 0.6, cursor: "not-allowed" },
              })}
            >
              {events.isFetchingNextPage && (
                <Loader2 size={15} className={css({ animation: "spin 0.9s linear infinite" })} />
              )}
              Load more
            </button>
          )}
        </div>
      )}
    </div>
  );
}

function EventRow({ event, stacks }: { event: LogEvent; stacks: Stack[] }) {
  const meta = EVENT_META[event.event] ?? {
    label: "Event",
    icon: ScrollText,
    bg: "ink.100",
    fg: "textMuted",
  };
  const Icon = meta.icon;
  const serviceId = resolveServiceId(event, stacks);

  return (
    <div
      className={flex({
        align: "center",
        gap: "3",
        px: "4",
        py: "2.5",
        borderBottomWidth: "1px",
        borderColor: "border",
        transition: "background 0.12s ease",
        _hover: { bg: "bg" },
        _last: { borderBottomWidth: "0" },
      })}
    >
      <span
        className={flex({
          align: "center",
          justify: "center",
          w: "6",
          h: "6",
          borderRadius: "md",
          bg: meta.bg,
          color: meta.fg,
          flexShrink: 0,
        })}
        title={meta.label}
      >
        <Icon size={14} />
      </span>
      <span
        className={css({
          fontWeight: "bold",
          fontSize: "sm",
          color: "text",
          whiteSpace: "nowrap",
          flexShrink: 0,
        })}
      >
        {event.title}
      </span>
      {event.body && (
        <span
          className={css({
            flex: "1",
            minW: "0",
            fontSize: "sm",
            color: "textMuted",
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          })}
          title={event.body}
        >
          {event.body}
        </span>
      )}
      <span className={hstack({ gap: "3", ml: "auto", flexShrink: 0 })}>
        {serviceId && (
          <Link
            to="/service/$id"
            params={{ id: serviceId }}
            className={css({
              fontSize: "xs",
              fontWeight: "bold",
              color: "grape.600",
              textDecoration: "none",
              maxW: "40",
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
              display: { base: "none", sm: "block" },
              _hover: { textDecoration: "underline" },
            })}
          >
            {event.containerName || serviceId}
          </Link>
        )}
        <time
          className={css({ fontSize: "xs", color: "textMuted", w: "16", textAlign: "right" })}
          dateTime={new Date(event.createdAt * 1000).toISOString()}
        >
          {formatTime(event.createdAt)}
        </time>
      </span>
    </div>
  );
}

/** Resolve the stack id a row links to: its own stackId, or the stack whose
 * containers include the named one. Empty when neither is known. */
function resolveServiceId(event: LogEvent, stacks: Stack[]): string {
  if (event.stackId) return event.stackId;
  if (!event.containerName) return "";
  for (const s of stacks) {
    for (const c of s.containers) {
      if (c.containerName === event.containerName) return s.id;
    }
  }
  return "";
}

function matchesSearch(event: LogEvent, q: string): boolean {
  const needle = q.toLowerCase();
  return (
    event.title.toLowerCase().includes(needle) ||
    event.body.toLowerCase().includes(needle)
  );
}

interface DayGroup {
  day: string;
  items: LogEvent[];
}

function groupByDay(items: LogEvent[]): DayGroup[] {
  const groups: DayGroup[] = [];
  let current: DayGroup | null = null;
  for (const ev of items) {
    const day = formatDay(ev.createdAt);
    if (!current || current.day !== day) {
      current = { day, items: [] };
      groups.push(current);
    }
    current.items.push(ev);
  }
  return groups;
}

function useDebounced<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value);
  const timer = useRef<number>(0);
  useEffect(() => {
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setDebounced(value), delayMs);
    return () => window.clearTimeout(timer.current);
  }, [value, delayMs]);
  return debounced;
}

function formatDay(seconds: number): string {
  const date = new Date(seconds * 1000);
  const today = new Date();
  const yesterday = new Date();
  yesterday.setDate(today.getDate() - 1);
  if (sameDay(date, today)) return "Today";
  if (sameDay(date, yesterday)) return "Yesterday";
  return date.toLocaleDateString(undefined, {
    weekday: "long",
    month: "short",
    day: "numeric",
    year: date.getFullYear() === today.getFullYear() ? undefined : "numeric",
  });
}

function sameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

function formatTime(seconds: number): string {
  return new Date(seconds * 1000).toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function Spinner() {
  return (
    <div className={flex({ justify: "center", py: "6" })}>
      <Loader2 size={28} className={css({ color: "accent", animation: "spin 0.9s linear infinite" })} />
    </div>
  );
}

function EmptyNote({ children }: { children: React.ReactNode }) {
  return <p className={css({ color: "textMuted", fontWeight: "medium" })}>{children}</p>;
}
