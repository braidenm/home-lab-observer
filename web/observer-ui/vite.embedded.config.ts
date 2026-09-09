import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  root: "embedded",
  base: "/",
  plugins: [react()],
  build: {
    outDir: "../../../internal/webui/assets",
    emptyOutDir: true,
    sourcemap: false,
    assetsInlineLimit: 0,
    modulePreload: false,
    cssCodeSplit: false,
    rollupOptions: {
      output: {
        entryFileNames: "static/dashboard.js",
        chunkFileNames: "static/[name].js",
        assetFileNames: (asset) => asset.names.some((name) => name.endsWith(".css"))
          ? "static/dashboard.css"
          : "static/[name][extname]"
      }
    }
  }
});
