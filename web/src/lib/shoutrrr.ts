// Shoutrrr service URL specs. Field slots mirror the `url:` struct tags in
// github.com/containrrr/shoutrrr/pkg/services/*, so a built URL parses back into
// the same config on the Go side.

export type Slot =
  | "user"
  | "pass"
  | "host"
  | "port"
  | "path"
  | "path1"
  | "path2"
  | "path3"
  | "query";

export type FieldSpec = {
  name: string;
  label: string;
  slot: Slot;
  key?: string;
  required?: boolean;
  secret?: boolean;
  advanced?: boolean;
  placeholder?: string;
  hint?: string;
  options?: string[];
  /** Value holds an `id:secret` pair that the service reads from user and password. */
  colonPair?: boolean;
};

export type ServiceSpec = {
  scheme: string;
  label: string;
  docs: string;
  fields: FieldSpec[];
  /** Slots the service fixes to a literal, e.g. telegram's `@telegram` host. */
  constants?: Partial<Record<Slot, string>>;
  defaults?: Record<string, string>;
};

const DOCS = (name: string) => `https://containrrr.dev/shoutrrr/v0.8/services/${name}/`;

export const SERVICES: ServiceSpec[] = [
  {
    scheme: "discord",
    label: "Discord",
    docs: DOCS("discord"),
    fields: [
      {
        name: "token",
        label: "Webhook token",
        slot: "user",
        required: true,
        secret: true,
        hint: "The part after the last slash of the webhook URL.",
      },
      { name: "id", label: "Webhook ID", slot: "host", required: true },
      { name: "username", label: "Override username", slot: "query", key: "username", advanced: true },
      { name: "avatar", label: "Override avatar URL", slot: "query", key: "avatar", advanced: true },
    ],
  },
  {
    scheme: "ntfy",
    label: "ntfy",
    docs: DOCS("ntfy"),
    fields: [
      { name: "host", label: "Server", slot: "host", required: true, placeholder: "ntfy.sh" },
      { name: "topic", label: "Topic", slot: "path1", required: true },
      { name: "user", label: "Username", slot: "user", advanced: true },
      { name: "pass", label: "Password", slot: "pass", secret: true, advanced: true },
      {
        name: "priority",
        label: "Priority",
        slot: "query",
        key: "priority",
        advanced: true,
        options: ["", "min", "low", "default", "high", "max"],
      },
      {
        name: "tags",
        label: "Tags",
        slot: "query",
        key: "tags",
        advanced: true,
        hint: "Comma separated, may map to emoji.",
      },
      {
        name: "scheme",
        label: "Protocol",
        slot: "query",
        key: "scheme",
        advanced: true,
        options: ["", "https", "http"],
      },
    ],
    defaults: { host: "ntfy.sh" },
  },
  {
    scheme: "slack",
    label: "Slack",
    docs: DOCS("slack"),
    fields: [
      {
        name: "type",
        label: "Token type",
        slot: "user",
        required: true,
        options: ["hook", "xoxb"],
        hint: "hook for an incoming webhook, xoxb for a bot API token.",
      },
      {
        name: "token",
        label: "Token",
        slot: "pass",
        required: true,
        secret: true,
        placeholder: "T00000000-B00000000-XXXXXXXXXXXXXXXXXXXXXXXX",
        hint: "The three parts of the webhook URL, joined with dashes.",
      },
      {
        name: "channel",
        label: "Channel",
        slot: "host",
        required: true,
        placeholder: "C001CH4NN3L",
      },
      { name: "botname", label: "Bot name", slot: "query", key: "botname", advanced: true },
      { name: "color", label: "Border color", slot: "query", key: "color", advanced: true },
      {
        name: "icon",
        label: "Icon",
        slot: "query",
        key: "icon",
        advanced: true,
        hint: "Emoji, or a URL starting with http(s)://.",
      },
      {
        name: "thread_ts",
        label: "Thread timestamp",
        slot: "query",
        key: "thread_ts",
        advanced: true,
        hint: "ts of a parent message, to reply in its thread.",
      },
    ],
    defaults: { type: "hook" },
  },
  {
    scheme: "telegram",
    label: "Telegram",
    docs: DOCS("telegram"),
    constants: { host: "telegram" },
    fields: [
      {
        name: "token",
        label: "Bot token",
        slot: "user",
        required: true,
        secret: true,
        colonPair: true,
        placeholder: "110201543:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw",
      },
      {
        name: "chats",
        label: "Chats",
        slot: "query",
        key: "chats",
        required: true,
        hint: "Chat IDs, or channel names like @my-channel. Comma separated.",
      },
      {
        name: "parsemode",
        label: "Parse mode",
        slot: "query",
        key: "parsemode",
        advanced: true,
        options: ["", "None", "Markdown", "HTML", "MarkdownV2"],
      },
      {
        name: "notification",
        label: "Sound",
        slot: "query",
        key: "notification",
        advanced: true,
        options: ["", "yes", "no"],
      },
    ],
  },
  {
    scheme: "gotify",
    label: "Gotify",
    docs: DOCS("gotify"),
    fields: [
      { name: "host", label: "Server", slot: "host", required: true, placeholder: "gotify.example.com" },
      { name: "token", label: "App token", slot: "path1", required: true, secret: true },
      { name: "priority", label: "Priority", slot: "query", key: "priority", advanced: true },
      {
        name: "disabletls",
        label: "Plain HTTP",
        slot: "query",
        key: "disabletls",
        advanced: true,
        options: ["", "yes", "no"],
      },
    ],
  },
  {
    scheme: "pushover",
    label: "Pushover",
    docs: DOCS("pushover"),
    constants: { user: "shoutrrr" },
    fields: [
      { name: "token", label: "API token", slot: "pass", required: true, secret: true },
      { name: "userkey", label: "User key", slot: "host", required: true },
      {
        name: "devices",
        label: "Devices",
        slot: "query",
        key: "devices",
        advanced: true,
        hint: "Comma separated. Empty means all devices.",
      },
      { name: "priority", label: "Priority", slot: "query", key: "priority", advanced: true },
    ],
  },
  {
    scheme: "matrix",
    label: "Matrix",
    docs: DOCS("matrix"),
    fields: [
      { name: "host", label: "Server", slot: "host", required: true, placeholder: "matrix.org" },
      {
        name: "pass",
        label: "Password or access token",
        slot: "pass",
        required: true,
        secret: true,
      },
      {
        name: "user",
        label: "Username",
        slot: "user",
        hint: "Leave empty when using an access token.",
      },
      {
        name: "rooms",
        label: "Rooms",
        slot: "query",
        key: "rooms",
        hint: "Room aliases, or room IDs prefixed with !. Comma separated.",
      },
    ],
  },
  {
    scheme: "smtp",
    label: "Email (SMTP)",
    docs: DOCS("email"),
    fields: [
      { name: "host", label: "SMTP server", slot: "host", required: true },
      { name: "port", label: "Port", slot: "port", required: true, placeholder: "587" },
      { name: "from", label: "From address", slot: "query", key: "from", required: true },
      {
        name: "to",
        label: "To addresses",
        slot: "query",
        key: "to",
        required: true,
        hint: "Comma separated.",
      },
      { name: "user", label: "Username", slot: "user" },
      { name: "pass", label: "Password", slot: "pass", secret: true },
      {
        name: "auth",
        label: "Authentication",
        slot: "query",
        key: "auth",
        advanced: true,
        options: ["", "None", "Plain", "CRAMMD5", "OAuth2", "Unknown"],
      },
      {
        name: "starttls",
        label: "StartTLS",
        slot: "query",
        key: "starttls",
        advanced: true,
        options: ["", "yes", "no"],
      },
      {
        name: "encryption",
        label: "Encryption",
        slot: "query",
        key: "encryption",
        advanced: true,
        options: ["", "Auto", "None", "ExplicitTLS", "ImplicitTLS"],
      },
    ],
    defaults: { port: "587" },
  },
  {
    scheme: "generic",
    label: "Webhook",
    docs: DOCS("generic"),
    fields: [
      { name: "host", label: "Host", slot: "host", required: true, placeholder: "example.com" },
      { name: "path", label: "Path", slot: "path", placeholder: "/api/v1/notify" },
      { name: "user", label: "Username", slot: "user", advanced: true },
      { name: "pass", label: "Password", slot: "pass", secret: true, advanced: true },
      {
        name: "titlekey",
        label: "Title JSON key",
        slot: "query",
        key: "titlekey",
        advanced: true,
        placeholder: "title",
      },
      {
        name: "messagekey",
        label: "Message JSON key",
        slot: "query",
        key: "messagekey",
        advanced: true,
        placeholder: "message",
      },
      {
        name: "contenttype",
        label: "Content type",
        slot: "query",
        key: "contenttype",
        advanced: true,
        placeholder: "application/json",
      },
      {
        name: "method",
        label: "Method",
        slot: "query",
        key: "method",
        advanced: true,
        options: ["", "POST", "PUT", "PATCH", "GET"],
      },
      {
        name: "disabletls",
        label: "Plain HTTP",
        slot: "query",
        key: "disabletls",
        advanced: true,
        options: ["", "yes", "no"],
      },
    ],
  },
  {
    scheme: "bark",
    label: "Bark",
    docs: DOCS("bark"),
    fields: [
      { name: "devicekey", label: "Device key", slot: "user", required: true, secret: true },
      { name: "host", label: "Server", slot: "host", required: true },
      { name: "sound", label: "Sound", slot: "query", key: "sound", advanced: true },
      { name: "group", label: "Group", slot: "query", key: "group", advanced: true },
      {
        name: "scheme",
        label: "Protocol",
        slot: "query",
        key: "scheme",
        advanced: true,
        options: ["", "https", "http"],
      },
    ],
  },
  {
    scheme: "mattermost",
    label: "Mattermost",
    docs: DOCS("mattermost"),
    fields: [
      { name: "host", label: "Server", slot: "host", required: true },
      { name: "token", label: "Webhook token", slot: "path1", required: true, secret: true },
      { name: "channel", label: "Channel", slot: "path2" },
      { name: "user", label: "Override username", slot: "user", advanced: true },
      {
        name: "icon",
        label: "Icon",
        slot: "query",
        key: "icon",
        advanced: true,
        hint: "Emoji, or a URL starting with http(s)://.",
      },
    ],
  },
  {
    scheme: "rocketchat",
    label: "Rocket.Chat",
    docs: DOCS("rocketchat"),
    fields: [
      { name: "host", label: "Server", slot: "host", required: true },
      { name: "tokenA", label: "Token A", slot: "path1", required: true, secret: true },
      { name: "tokenB", label: "Token B", slot: "path2", required: true, secret: true },
      {
        name: "channel",
        label: "Channel or recipient",
        slot: "path3",
        hint: "A channel name, or @user for a direct message.",
      },
      { name: "user", label: "Override username", slot: "user", advanced: true },
    ],
  },
  {
    scheme: "teams",
    label: "Microsoft Teams",
    docs: DOCS("teams"),
    fields: [
      { name: "group", label: "Group", slot: "user", required: true },
      { name: "tenant", label: "Tenant", slot: "host", required: true },
      { name: "altid", label: "Alt ID", slot: "path1", required: true },
      { name: "groupowner", label: "Group owner", slot: "path2", required: true },
      {
        name: "whhost",
        label: "Webhook host",
        slot: "query",
        key: "host",
        required: true,
        placeholder: "organization.webhook.office.com",
      },
      { name: "color", label: "Color", slot: "query", key: "color", advanced: true },
    ],
  },
  {
    scheme: "zulip",
    label: "Zulip",
    docs: DOCS("zulip"),
    fields: [
      { name: "botmail", label: "Bot email", slot: "user", required: true },
      { name: "botkey", label: "Bot key", slot: "pass", required: true, secret: true },
      { name: "host", label: "Server", slot: "host", required: true },
      { name: "stream", label: "Stream", slot: "query", key: "stream", required: true },
      { name: "topic", label: "Topic", slot: "query", key: "topic" },
    ],
  },
  {
    scheme: "opsgenie",
    label: "OpsGenie",
    docs: DOCS("opsgenie"),
    fields: [
      {
        name: "host",
        label: "API host",
        slot: "host",
        required: true,
        placeholder: "api.opsgenie.com",
        hint: "Use api.eu.opsgenie.com for EU instances.",
      },
      { name: "apikey", label: "API key", slot: "path1", required: true, secret: true },
      {
        name: "responders",
        label: "Responders",
        slot: "query",
        key: "responders",
        hint: "Comma separated, e.g. team:ops.",
      },
      { name: "priority", label: "Priority", slot: "query", key: "priority", advanced: true },
      { name: "tags", label: "Tags", slot: "query", key: "tags", advanced: true },
    ],
    defaults: { host: "api.opsgenie.com" },
  },
  {
    scheme: "pushbullet",
    label: "Pushbullet",
    docs: DOCS("pushbullet"),
    fields: [
      { name: "token", label: "API token", slot: "host", required: true, secret: true },
      {
        name: "target",
        label: "Target",
        slot: "path1",
        hint: "A device, #channel, or email address. Empty sends to all devices.",
      },
    ],
  },
  {
    scheme: "join",
    label: "Join",
    docs: DOCS("join"),
    constants: { user: "shoutrrr", host: "join" },
    fields: [
      { name: "apikey", label: "API key", slot: "pass", required: true, secret: true },
      {
        name: "devices",
        label: "Devices",
        slot: "query",
        key: "devices",
        required: true,
        hint: "Device IDs, comma separated.",
      },
      { name: "icon", label: "Icon URL", slot: "query", key: "icon", advanced: true },
    ],
  },
  {
    scheme: "googlechat",
    label: "Google Chat",
    docs: DOCS("googlechat"),
    constants: { host: "chat.googleapis.com" },
    fields: [
      {
        name: "path",
        label: "Space path",
        slot: "path",
        required: true,
        placeholder: "/v1/spaces/FOO/messages",
      },
      { name: "key", label: "Key", slot: "query", key: "key", required: true, secret: true },
      { name: "token", label: "Token", slot: "query", key: "token", required: true, secret: true },
    ],
  },
  {
    scheme: "ifttt",
    label: "IFTTT",
    docs: DOCS("ifttt"),
    fields: [
      { name: "webhookid", label: "Webhook ID", slot: "host", required: true, secret: true },
      {
        name: "events",
        label: "Events",
        slot: "query",
        key: "events",
        required: true,
        hint: "Comma separated.",
      },
      { name: "value1", label: "Value 1", slot: "query", key: "value1", advanced: true },
      { name: "value2", label: "Value 2", slot: "query", key: "value2", advanced: true },
      { name: "value3", label: "Value 3", slot: "query", key: "value3", advanced: true },
    ],
  },
];

export const CUSTOM_SCHEME = "__custom";

export type Target = {
  id: string;
  scheme: string;
  values: Record<string, string>;
  /** Query parameters we have no field for, kept so an edit doesn't drop them. */
  extraQuery: Record<string, string>;
  /** Raw URL, for targets we cannot represent with a form. */
  raw: string;
};

export function serviceFor(scheme: string): ServiceSpec | undefined {
  return SERVICES.find((s) => s.scheme === scheme);
}

let counter = 0;
function nextID() {
  counter += 1;
  return `t${counter}`;
}

export function newTarget(scheme: string): Target {
  const spec = serviceFor(scheme);
  return {
    id: nextID(),
    scheme,
    values: { ...(spec?.defaults ?? {}) },
    extraQuery: {},
    raw: "",
  };
}

export function missingFields(t: Target): FieldSpec[] {
  const spec = serviceFor(t.scheme);
  if (!spec) return t.raw.trim() ? [] : [];
  return spec.fields.filter((f) => f.required && !(t.values[f.name] ?? "").trim());
}

export function isComplete(t: Target): boolean {
  if (t.scheme === CUSTOM_SCHEME) return t.raw.trim().length > 0;
  return missingFields(t).length === 0;
}

export function buildURL(t: Target): string {
  if (t.scheme === CUSTOM_SCHEME) return t.raw.trim();
  const spec = serviceFor(t.scheme);
  if (!spec) return t.raw.trim();

  const slot = (s: Slot): string => {
    const constant = spec.constants?.[s];
    if (constant !== undefined) return constant;
    const field = spec.fields.find((f) => f.slot === s);
    return field ? (t.values[field.name] ?? "").trim() : "";
  };

  let user = slot("user");
  let pass = slot("pass");
  const host = slot("host");
  const port = slot("port");

  const pairField = spec.fields.find((f) => f.colonPair);
  if (pairField) {
    const [head, ...rest] = (t.values[pairField.name] ?? "").trim().split(":");
    if (pairField.slot === "user") {
      user = head;
      pass = rest.join(":");
    }
  }

  let authority = "";
  if (user || pass) {
    authority = enc(user);
    if (pass) authority += `:${enc(pass)}`;
    authority += "@";
  }
  authority += host;
  if (port) authority += `:${port}`;

  const segments: string[] = [];
  const whole = slot("path");
  if (whole) {
    for (const part of whole.split("/")) if (part) segments.push(enc(part));
  }
  for (const s of ["path1", "path2", "path3"] as const) {
    const v = slot(s);
    if (v) segments.push(enc(v));
  }
  const path = segments.length ? `/${segments.join("/")}` : "";

  const query = new URLSearchParams();
  for (const [k, v] of Object.entries(t.extraQuery)) if (v) query.set(k, v);
  for (const f of spec.fields) {
    if (f.slot !== "query" || !f.key) continue;
    const v = (t.values[f.name] ?? "").trim();
    if (v) query.set(f.key, v);
    else query.delete(f.key);
  }
  const qs = query.toString();

  return `${spec.scheme}://${authority}${path}${qs ? `?${qs}` : ""}`;
}

function enc(v: string): string {
  return encodeURIComponent(v);
}

function dec(v: string): string {
  try {
    return decodeURIComponent(v);
  } catch {
    return v;
  }
}

// Hand-rolled instead of `new URL`, which lowercases the host — Slack and Teams
// carry case-sensitive tokens there.
const URL_RE = /^([a-z][a-z0-9+.-]*):\/\/(?:([^:@/?#]*)(?::([^@/?#]*))?@)?([^/?#]*)(\/[^?#]*)?(?:\?([^#]*))?$/i;

export function parseURL(url: string): Target {
  const raw = url.trim();
  const custom = (): Target => ({
    id: nextID(),
    scheme: CUSTOM_SCHEME,
    values: {},
    extraQuery: {},
    raw,
  });

  const m = URL_RE.exec(raw);
  if (!m) return custom();
  const [, scheme, user, pass, hostPort, path, query] = m;
  const spec = serviceFor(scheme.toLowerCase());
  if (!spec) return custom();

  let host = hostPort ?? "";
  let port = "";
  const colon = host.lastIndexOf(":");
  if (colon !== -1) {
    port = host.slice(colon + 1);
    host = host.slice(0, colon);
  }

  const segments = (path ?? "").split("/").filter(Boolean).map(dec);
  const params = new URLSearchParams(query ?? "");

  const t: Target = { id: nextID(), scheme: spec.scheme, values: {}, extraQuery: {}, raw };
  const pathFields = spec.fields.filter((f) => f.slot.startsWith("path") && f.slot !== "path");
  const wholePathField = spec.fields.find((f) => f.slot === "path");

  for (const f of spec.fields) {
    switch (f.slot) {
      case "user":
        t.values[f.name] =
          f.colonPair && pass ? `${dec(user ?? "")}:${dec(pass)}` : dec(user ?? "");
        break;
      case "pass":
        t.values[f.name] = dec(pass ?? "");
        break;
      case "host":
        t.values[f.name] = host;
        break;
      case "port":
        t.values[f.name] = port;
        break;
      case "path":
        break;
      case "path1":
      case "path2":
      case "path3": {
        const idx = Number(f.slot.slice(4)) - 1;
        t.values[f.name] = segments[idx] ?? "";
        break;
      }
      case "query":
        t.values[f.name] = f.key ? (params.get(f.key) ?? "") : "";
        break;
    }
  }

  if (wholePathField) {
    // Whole-path services take every segment the indexed fields did not claim.
    const rest = segments.slice(0, segments.length - pathFields.length || undefined);
    t.values[wholePathField.name] = rest.length ? `/${rest.join("/")}` : "";
  }

  const known = new Set(
    spec.fields.filter((f) => f.slot === "query" && f.key).map((f) => f.key as string),
  );
  for (const [k, v] of params.entries()) if (!known.has(k)) t.extraQuery[k] = v;

  // If the URL does not round-trip, we would silently rewrite the user's target
  // on save. Fall back to editing it as raw text.
  if (canonical(buildURL(t)) !== canonical(raw)) return custom();
  return t;
}

/** Re-encoded, query-sorted form, so a round-trip check ignores harmless spelling differences. */
function canonical(url: string): string {
  const m = URL_RE.exec(url.trim());
  if (!m) return url.trim();
  const [, scheme, user, pass, hostPort, path, query] = m;
  const auth = user || pass ? `${enc(dec(user ?? ""))}:${enc(dec(pass ?? ""))}@` : "";
  const segments = (path ?? "")
    .split("/")
    .filter(Boolean)
    .map((s) => enc(dec(s)))
    .join("/");
  const params = [...new URLSearchParams(query ?? "").entries()].sort(([a], [b]) =>
    a.localeCompare(b),
  );
  const qs = params.map(([k, v]) => `${k}=${enc(v)}`).join("&");
  return `${scheme.toLowerCase()}://${auth}${hostPort ?? ""}${segments ? `/${segments}` : ""}${qs ? `?${qs}` : ""}`;
}
