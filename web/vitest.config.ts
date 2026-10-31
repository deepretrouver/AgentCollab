import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  test: {
    // Keep jsdom workers bounded on shared developer laptops and CI hosts.
    // Override with --maxWorkers when deliberately measuring higher concurrency.
    maxWorkers: 2,
    environment: "jsdom",
    globals: true,
    setupFiles: "./src/test/setup.ts",
  },
});
