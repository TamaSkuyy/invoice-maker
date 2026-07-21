import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",           // simulate browser DOM
    setupFiles: ["./src/test/setup.ts"], // global test setup
    globals: true,                   // expect, describe, it tanpa import
    css: false,                      // skip CSS parsing (lebih cepat)
    exclude: ["e2e/**", "node_modules/**"],  // E2E tests pakai Playwright, bukan Vitest
  },
});
