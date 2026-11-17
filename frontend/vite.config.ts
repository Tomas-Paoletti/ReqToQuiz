import { writeFileSync } from "node:fs";
import { fileURLToPath, URL } from "node:url";
import vue from "@vitejs/plugin-vue";
import { defineConfig, type Plugin } from "vite";

const outDir = fileURLToPath(new URL("../backend/web/dist", import.meta.url));

// Vite empties outDir on build; keep the placeholder that lets `go build`
// compile (go:embed needs the folder) even before the frontend is built.
const keepGitkeep: Plugin = {
  name: "keep-gitkeep",
  closeBundle() {
    writeFileSync(`${outDir}/.gitkeep`, "");
  },
};

const backend = "http://localhost:8080";

export default defineConfig({
  plugins: [vue(), keepGitkeep],
  build: { outDir, emptyOutDir: true },
  server: {
    port: 5173,
    proxy: {
      "/api": { target: backend, changeOrigin: true },
      "/quizzes": { target: backend, changeOrigin: true },
    },
  },
});
