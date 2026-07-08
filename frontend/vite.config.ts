import { reactRouter } from "@react-router/dev/vite";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig, loadEnv } from "vite";
import tsconfigPaths from "vite-tsconfig-paths";

export default defineConfig(({ mode }) => {
  // Where the Go API is running during local dev. Override with
  // VITE_API_TARGET (e.g. http://localhost:9000) if you changed PORT.
  const env = loadEnv(mode, process.cwd(), "");
  const apiTarget = env.VITE_API_TARGET ?? "http://localhost:8000";

  return {
    plugins: [tailwindcss(), reactRouter(), tsconfigPaths()],
    server: {
      // Forward same-origin API calls (/v1/...) to the Go binary so the
      // frontend dev server keeps HMR while talking to the real API.
      // Cookies keep working because the browser still sees everything on
      // http://localhost:5173 (same origin).
      proxy: {
        "/v1": {
          target: apiTarget,
          changeOrigin: true,
        },
      },
    },
  };
});
