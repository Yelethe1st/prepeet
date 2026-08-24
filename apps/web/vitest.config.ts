import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

/**
 * Test configuration for the web application.
 *
 * PLT-10 requires the frontend to be held to the same standard as the services,
 * so coverage thresholds fail the run here exactly as they do in Go and Python.
 */
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
      // The generated contract types, aliased rather than made a workspace
      // package: openapi-typescript emits interfaces and nothing else, so a
      // package would ship no runtime and exist only to be resolved.
      "@contracts": fileURLToPath(
        new URL("../../packages/generated/typescript/api.gen.ts", import.meta.url),
      ),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./vitest.setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    coverage: {
      provider: "v8",
      reporter: ["text", "lcov"],
      include: ["src/**/*.{ts,tsx}"],
      // Route files are Next.js composition points with no logic of their own,
      // and type declarations have nothing to execute.
      exclude: [
        "src/**/*.test.{ts,tsx}",
        "src/**/*.d.ts",
        "src/app/**/layout.tsx",
        "src/app/**/page.tsx",
      ],
      thresholds: { lines: 80, functions: 80, branches: 80, statements: 80 },
    },
  },
});
