import {
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
} from "@tanstack/react-router";
import { AppShell } from "./components/AppShell";
import { Dashboard } from "./routes/Dashboard";
import { Login } from "./routes/Login";
import { Enroll } from "./routes/Enroll";
import { Settings } from "./routes/Settings";
import { Invites } from "./routes/Invites";
import { ServiceDetail } from "./routes/ServiceDetail";

const rootRoute = createRootRoute({
  component: () => (
    <AppShell>
      <Outlet />
    </AppShell>
  ),
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
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
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: Settings,
});

const invitesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/invites",
  component: Invites,
});

const serviceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/service/$id",
  component: function ServiceRoute() {
    const { id } = serviceRoute.useParams();
    return <ServiceDetail id={id} />;
  },
});

const routeTree = rootRoute.addChildren([
  indexRoute,
  loginRoute,
  enrollRoute,
  settingsRoute,
  invitesRoute,
  serviceRoute,
]);

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
