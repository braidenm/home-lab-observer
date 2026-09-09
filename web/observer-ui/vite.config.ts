import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: true,
    lib: {
      entry: {
        index: "src/index.ts",
        local: "src/local.ts",
        demo: "src/demo.ts"
      },
      name: "HomeLabObserverUI",
      formats: ["es"],
      fileName: (_format, entryName) => `${entryName}.js`,
      cssFileName: "observer-ui"
    },
    rollupOptions: {
      external: ["react", "react-dom", "react/jsx-runtime"]
    }
  }
});
