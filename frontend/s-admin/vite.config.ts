import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

// Dev-only mirror of deploy/nginx/conf.d/20-s-admin.conf's /api rewrite,
// so `npm run dev` talks to a locally running API the same way
// production nginx does.
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5174,
    proxy: {
      "/api/public/auth/login": {
        target: "http://localhost:8080",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api\/public\/auth\/login/, "/public/auth/login"),
      },
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, "/s-admin"),
      },
    },
  },
  build: { outDir: "dist" },
});
