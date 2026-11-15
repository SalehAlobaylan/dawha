import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  // maplibre-gl resolves its tile worker from `import.meta.url`. Vite's
  // dependency pre-bundling copies the entry but not the sibling worker file,
  // so `new Worker(...)` 404s and every GeoJSON layer silently never paints —
  // only DOM canvases/markers survive. Excluding maplibre from pre-bundling
  // keeps its own ESM graph and its worker intact.
  optimizeDeps: {
    exclude: ["maplibre-gl"],
  },
  server: {
    port: 5173,
    host: "0.0.0.0",
  },
  // `npm run preview` serves the production bundle with no VITE_API_URL of its
  // own: proxy the same-origin /api calls to the documented local default so the
  // preview is usable with zero env. Production itself still decides its API
  // honestly — a built bundle never guesses; this only covers the local preview.
  preview: {
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
