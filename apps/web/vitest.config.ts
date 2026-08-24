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
      "@contracts/capabilities": fileURLToPath(
        new URL("../../packages/generated/typescript/capabilities.gen.ts", import.meta.url),
      ),
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
      // Route files were excluded here as "composition points with no logic of
      // their own". That was true when written and stopped being true the
      // moment a page decided where to navigate after signing in, and because
      // the exclusion was blanket, nothing said so. Pages are covered now.
      //
      // The route group layout stays out: it wraps children in a div and picks
      // stylesheets, and there is nothing in it to assert that rendering a page
      // does not already cover. The root layout is covered, because it decides
      // the theme the browser sees before any React runs.
      exclude: [
        "src/**/*.test.{ts,tsx}",
        "src/**/*.d.ts",
                // Barrel files re-export and do nothing.
        "src/shared/components/index.ts",
      ],
      // Raised from 80 as the suite grew. A floor well below where the suite
      // actually sits is a floor that permits a large regression without
      // failing, which is the opposite of what it is for.
      thresholds: { lines: 95, functions: 95, branches: 90, statements: 95 },
    },
  },
});
