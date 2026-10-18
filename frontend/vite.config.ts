import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// In dev, Vite serves the UI on :5173 and forwards /api to the Go server.
// The browser sees a single origin, so session cookies just work.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    // Explicit IPv4: with "localhost" Node on Windows may bind only to ::1,
    // which some browsers/VPN clients don't reach. Fail instead of silently
    // moving to another port if 5173 is taken.
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
