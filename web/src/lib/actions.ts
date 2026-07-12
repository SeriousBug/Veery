import { http, HttpError } from "../api/http";
import { toaster } from "./toaster";
import type { SetAutoUpdateRequest } from "../api/generated";

function enc(id: string): string {
  return encodeURIComponent(id);
}

/**
 * Fire a mutation. The real progress arrives over the WS "job" stream, so on
 * success we stay quiet and let that drive feedback; we only surface a toast if
 * the request itself fails to reach or is rejected by the backend.
 */
async function fire(
  path: string,
  failTitle: string,
  body?: Record<string, unknown>,
): Promise<boolean> {
  try {
    await http.post(path, body);
    return true;
  } catch (err) {
    const message =
      err instanceof HttpError ? err.message : "Could not reach the server.";
    toaster.create({ type: "error", title: failTitle, description: message });
    return false;
  }
}

export type StackAction = "start" | "stop" | "restart" | "bringup" | "adopt";

export function stackAction(id: string, action: StackAction): Promise<boolean> {
  return fire(`/api/stacks/${enc(id)}/${action}`, "That didn't work");
}

export type ContainerAction = "start" | "stop" | "restart" | "update";

export function containerAction(
  id: string,
  action: ContainerAction,
): Promise<boolean> {
  return fire(`/api/containers/${enc(id)}/${action}`, "That didn't work");
}

export async function setAutoUpdate(
  containerId: string,
  autoUpdate: boolean,
): Promise<boolean> {
  const body: SetAutoUpdateRequest = { containerId, autoUpdate };
  return fire("/api/containers/autoupdate", "Could not change auto-update", {
    ...body,
  });
}
