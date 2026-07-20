import {
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
} from "@tanstack/react-router";
import type { ComponentType } from "react";
import { AppShell } from "./components/AppShell";
import { AuthProvider } from "./auth/AuthProvider";
import { RequireAuth } from "./auth/RequireAuth";
import { Dashboard } from "./routes/Dashboard";
import { Login } from "./routes/Login";
import { Enroll } from "./routes/Enroll";
import { Settings } from "./routes/Settings";
import { Invites } from "./routes/Invites";
import { Events } from "./routes/Events";
import { ServiceDetail } from "./routes/ServiceDetail";

function protectedPage<P extends object>(Page: ComponentType<P>) {
  return function Guarded(props: P) {
    return (
      <RequireAuth>
        <AppShell>
          <Page {...props} />
        </AppShell>
      </RequireAuth>
    );
  };
}

const rootRoute = createRootRoute({
  component: () => (
    <AuthProvider>
      <Outlet />
    </AuthProvider>
  ),
});

const ProtectedDashboard = protectedPage(Dashboard);
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: ProtectedDashboard,
});

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  component: Login,
});

const enrollRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/enroll",
  validateSearch: (search: Record<string, unknown>): { token: string } => ({
    token: typeof search.token === "string" ? search.token : "",
  }),
  component: function EnrollRoute() {
    const { token } = enrollRoute.useSearch();
    return <Enroll token={token} />;
  },
});

const ProtectedSettings = protectedPage(Settings);
const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: ProtectedSettings,
});

const ProtectedInvites = protectedPage(Invites);
const invitesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/invites",
  component: ProtectedInvites,
});

const ProtectedEvents = protectedPage(Events);
const eventsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/events",
  component: ProtectedEvents,
});

const ProtectedServiceDetail = protectedPage(ServiceDetail);
const serviceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/service/$id",
  component: function ServiceRoute() {
    const { id } = serviceRoute.useParams();
    return <ProtectedServiceDetail id={id} />;
  },
});

const routeTree = rootRoute.addChildren([
  indexRoute,
  loginRoute,
  enrollRoute,
  settingsRoute,
  invitesRoute,
  eventsRoute,
  serviceRoute,
]);

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
