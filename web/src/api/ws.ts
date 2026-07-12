import type { WSMessage, WSMessageType } from "./generated";

type Listener = (msg: WSMessage) => void;

type Unsubscribe = () => void;

interface WSClientOptions {
  /** Path on the same origin, proxied to the backend in dev. */
  path?: string;
  /** Reconnect backoff ceiling in ms. */
  maxBackoffMs?: number;
}

/**
 * Typed WebSocket client for the server→client push stream. Parses each frame
 * into a WSMessage and fans it out to subscribers. Reconnects with backoff.
 */
export class WSClient {
  private readonly url: string;
  private readonly maxBackoffMs: number;
  private socket: WebSocket | null = null;
  private backoffMs = 500;
  private closedByUser = false;
  private readonly all = new Set<Listener>();
  private readonly byType = new Map<WSMessageType, Set<Listener>>();

  constructor(opts: WSClientOptions = {}) {
    const path = opts.path ?? "/ws";
    this.maxBackoffMs = opts.maxBackoffMs ?? 15_000;
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    this.url = `${proto}//${location.host}${path}`;
  }

  connect(): void {
    this.closedByUser = false;
    this.open();
  }

  private open(): void {
    const socket = new WebSocket(this.url);
    this.socket = socket;

    socket.onopen = () => {
      this.backoffMs = 500;
    };

    socket.onmessage = (event: MessageEvent<string>) => {
      let msg: WSMessage;
      try {
        msg = JSON.parse(event.data) as WSMessage;
      } catch {
        return;
      }
      this.dispatch(msg);
    };

    socket.onclose = () => {
      this.socket = null;
      if (this.closedByUser) return;
      const wait = this.backoffMs;
      this.backoffMs = Math.min(this.backoffMs * 2, this.maxBackoffMs);
      setTimeout(() => {
        if (!this.closedByUser) this.open();
      }, wait);
    };
  }

  private dispatch(msg: WSMessage): void {
    for (const fn of this.all) fn(msg);
    const typed = this.byType.get(msg.type);
    if (typed) for (const fn of typed) fn(msg);
  }

  /** Subscribe to every message, or to one WSMessageType. */
  subscribe(listener: Listener): Unsubscribe;
  subscribe(type: WSMessageType, listener: Listener): Unsubscribe;
  subscribe(
    a: WSMessageType | Listener,
    b?: Listener,
  ): Unsubscribe {
    if (typeof a === "function") {
      this.all.add(a);
      return () => this.all.delete(a);
    }
    const listener = b as Listener;
    let set = this.byType.get(a);
    if (!set) {
      set = new Set();
      this.byType.set(a, set);
    }
    set.add(listener);
    return () => set.delete(listener);
  }

  close(): void {
    this.closedByUser = true;
    this.socket?.close();
    this.socket = null;
  }
}

export const wsClient = new WSClient();
