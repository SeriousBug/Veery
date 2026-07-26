import {
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
} from "@tanstack/react-router";
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

const rootRoute = createRootRoute({
  component: () => (
    <AuthProvider>
      <Outlet />
    </AuthProvider>
  ),
});

// Pathless layout route: the shell (and with it the push-stream connection in
// LiveDataProvider) stays mounted across navigations between protected pages.
const protectedLayout = createRoute({
  getParentRoute: () => rootRoute,
  id: "protected",
  component: () => (
    <RequireAuth>
      <AppShell>
        <Outlet />
      </AppShell>
    </RequireAuth>
  ),
});

const indexRoute = createRoute({
  getParentRoute: () => protectedLayout,
  path: "/",
  component: Dashboard,
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

const settingsRoute = createRoute({
  getParentRoute: () => protectedLayout,
  path: "/settings",
  component: Settings,
});

const invitesRoute = createRoute({
  getParentRoute: () => protectedLayout,
  path: "/invites",
  component: Invites,
});

const eventsRoute = createRoute({
  getParentRoute: () => protectedLayout,
  path: "/events",
  component: Events,
});

const serviceRoute = createRoute({
  getParentRoute: () => protectedLayout,
  path: "/service/$id",
  component: function ServiceRoute() {
    const { id } = serviceRoute.useParams();
    return <ServiceDetail id={id} />;
  },
});

const routeTree = rootRoute.addChildren([
  protectedLayout.addChildren([
    indexRoute,
    settingsRoute,
    invitesRoute,
    eventsRoute,
    serviceRoute,
  ]),
  loginRoute,
  enrollRoute,
]);

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
