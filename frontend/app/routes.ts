import { type RouteConfig, index, layout, route } from "@react-router/dev/routes";

export default [
  index("routes/home.tsx"),
  route("auth", "routes/auth.tsx"),

  layout("routes/dashboard.tsx", [
    route("app", "routes/dashboard.links.tsx"),
    route("app/feed", "routes/dashboard.feed.tsx"),
    route("app/library", "routes/dashboard.library.tsx"),
    route("app/read/:id", "routes/dashboard.read.tsx"),
  ]),
] satisfies RouteConfig;
