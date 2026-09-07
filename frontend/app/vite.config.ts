import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

// Dev-only mirror of deploy/nginx/conf.d/50-app.conf's /api rewrite, so
// `npm run dev` talks to a locally running API (e.g. `docker compose
// -f docker-compose.local.yml up`) the same way production nginx does.
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5177,
    proxy: {
      "/api/auth/login": {
        target: "http://localhost:8080",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api\/auth\/login/, "/public/auth/login"),
      },
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, "/family"),
      },
    },
  },
  build: { outDir: "dist" },
});
