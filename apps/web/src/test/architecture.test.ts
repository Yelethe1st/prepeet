import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { describe, expect, it } from "vitest";

/**
 * The boundaries the READMEs in this tree describe, enforced.
 *
 * They were prose until this file existed, and prose does not fail a build. Two
 * of them were already broken when it was written: the shell imported the auth
 * feature's types, and lib's README claimed to own a session provider that lives
 * in a feature and renders.
 *
 * The Go side has the same test for the same reason, and this is deliberately
 * its counterpart rather than something cleverer.
 */

const root = resolve(process.cwd(), "src");

/** Every source file under a directory, tests included. */
function sourceFiles(directory: string): string[] {
  const found: string[] = [];

  const walk = (path: string) => {
    for (const entry of readdirSync(path)) {
      const full = join(path, entry);
      if (statSync(full).isDirectory()) {
        walk(full);
        continue;
      }
      if (/\.(ts|tsx)$/.test(entry)) found.push(full);
    }
  };

  walk(directory);
  return found;
}

/** The module specifiers a file imports. */
function imports(file: string): string[] {
  const source = readFileSync(file, "utf8");
  return [...source.matchAll(/from\s+["']([^"']+)["']/g)].map((match) => match[1] as string);
}

function named(file: string): string {
  return relative(root, file);
}

/** The feature directories, discovered rather than listed. */
const features = readdirSync(join(root, "features")).filter((entry) =>
  statSync(join(root, "features", entry)).isDirectory(),
);

describe("boundaries", () => {
  it("has features to check, so this is not vacuous", () => {
    expect(features.length).toBeGreaterThan(1);
  });

  /**
   * A feature importing another feature is how two features become one, and the
   * cost is paid later, when one of them has to be changed and the other
   * breaks.
   *
   * What a feature needs from elsewhere it declares itself, and the route
   * supplies it. `app/` is the composition root and is allowed to see
   * everything, the way `cmd/` is on the server.
   */
  it("keeps every feature out of every other feature", () => {
    const crossings: string[] = [];

    for (const feature of features) {
      for (const file of sourceFiles(join(root, "features", feature))) {
        for (const specifier of imports(file)) {
          const match = /^@\/features\/([^/]+)/.exec(specifier);
          if (match && match[1] !== feature) {
            crossings.push(`${named(file)} imports ${specifier}`);
          }
        }
      }
    }

    expect(crossings).toEqual([]);
  });

  /**
   * The design system knows nothing about the product. A component that
   * imported a feature would be a component that cannot be used by any other
   * one, which is the opposite of what a design system is.
   */
  it("keeps the product out of shared", () => {
    const crossings: string[] = [];

    for (const file of sourceFiles(join(root, "shared"))) {
      for (const specifier of imports(file)) {
        if (specifier.startsWith("@/features/") || specifier.startsWith("@/app/")) {
          crossings.push(`${named(file)} imports ${specifier}`);
        }
      }
    }

    expect(crossings).toEqual([]);
  });

  /**
   * lib is what every feature needs and none of them owns, so it cannot need
   * any of them.
   */
  it("keeps features and routes out of lib", () => {
    const crossings: string[] = [];

    for (const file of sourceFiles(join(root, "lib"))) {
      for (const specifier of imports(file)) {
        if (specifier.startsWith("@/features/") || specifier.startsWith("@/app/")) {
          crossings.push(`${named(file)} imports ${specifier}`);
        }
      }
    }

    expect(crossings).toEqual([]);
  });

  /**
   * lib is plumbing, not presentation.
   *
   * An earlier version of this forbade any .tsx under lib at all, which was the
   * wrong rule: the architecture brief puts auth in lib, and knowing who is
   * signed in is a context provider, which is a component. A provider renders
   * its children and nothing of its own.
   *
   * What actually distinguishes plumbing from presentation is whether it
   * reaches for the design system. Something in lib that imported a Button is
   * something that should have been a feature.
   */
  it("presents nothing in lib", () => {
    const presenting: string[] = [];

    for (const file of sourceFiles(join(root, "lib"))) {
      for (const specifier of imports(file)) {
        if (specifier.startsWith("@/shared")) {
          presenting.push(`${named(file)} imports ${specifier}`);
        }
      }
    }

    expect(presenting).toEqual([]);
  });

  /**
   * A route composes; it does not implement. A page holding a form's logic is a
   * page that cannot be tested without a router, which is how route files end
   * up untested and then excluded from coverage for being untestable.
   */
  it("keeps routes thin", () => {
    const heavy: string[] = [];

    for (const file of sourceFiles(join(root, "app"))) {
      if (file.endsWith(".test.tsx") || file.endsWith(".test.ts")) continue;

      const lines = readFileSync(file, "utf8")
        .split("\n")
        .filter((line) => line.trim() !== "" && !line.trim().startsWith("*") && !line.trim().startsWith("//"))
        .filter((line) => !line.trim().startsWith("/*"));

      // Generous, and a ceiling rather than a target. A route longer than this
      // is composing something that should have been a component.
      if (lines.length > 80) {
        heavy.push(`${named(file)} has ${lines.length} lines of code`);
      }
    }

    expect(heavy).toEqual([]);
  });

  /**
   * Every directory under src explains what it owns and what it must never do.
   * A directory nobody described is a directory anybody can put anything in.
   */
  it("describes every area of the tree", () => {
    const undescribed = ["app", "features", "lib", "shared", "test"].filter((area) => {
      try {
        return readFileSync(join(root, area, "README.md"), "utf8").trim() === "";
      } catch {
        return true;
      }
    });

    expect(undescribed).toEqual([]);
  });
});
