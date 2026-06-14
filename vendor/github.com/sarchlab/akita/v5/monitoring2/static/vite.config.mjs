import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

const monitorTarget = process.env.AKITA_MONITOR_URL ?? "http://localhost:32776";

export default defineConfig({
  plugins: [react()],
  build: {
    sourcemap: true,
  },
  server: {
    host: "localhost",
    proxy: {
      "/api": {
        target: monitorTarget,
        changeOrigin: true,
      },
    },
  },
});
