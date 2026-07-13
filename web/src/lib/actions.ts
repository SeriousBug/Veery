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

/**
 * Stop managing a container or a whole service. The container is already gone
 * from the host, so there is no job to follow: the stacks push that follows
 * makes it disappear, and the toast says why.
 */
async function forget(path: string, name: string): Promise<boolean> {
  try {
    await http.del(path);
    toaster.create({
      type: "success",
      title: `Veery is no longer tracking ${name}`,
    });
    return true;
  } catch (err) {
    const message =
      err instanceof HttpError ? err.message : "Could not reach the server.";
    toaster.create({
      type: "error",
      title: "Could not forget it",
      description: message,
    });
    return false;
  }
}

export function forgetContainer(id: string, name: string): Promise<boolean> {
  return forget(`/api/containers/${enc(id)}/managed`, name);
}

export function forgetStack(id: string, name: string): Promise<boolean> {
  return forget(`/api/stacks/${enc(id)}/managed`, name);
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
