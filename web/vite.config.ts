import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The browser only talks to this server, which forwards API calls. That keeps
// the backends free of CORS rules and hides their addresses from the frontend.
function proxyTo(target: string, prefix: string) {
  return {
    target,
    changeOrigin: true,
    rewrite: (path: string) => path.slice(prefix.length),
  };
}

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api/engine": proxyTo(process.env.ENGINE_URL ?? "http://localhost:8080", "/api/engine"),
      "/api/coach": proxyTo(process.env.COACH_URL ?? "http://localhost:8000", "/api/coach"),
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test-setup.ts",
  },
});
