import * as axeMatchers from "vitest-axe/matchers";
import { expect } from "vitest";

import "@testing-library/jest-dom/vitest";

/**
 * Test setup for the web application.
 *
 * The axe matchers are registered here rather than per file. vitest-axe was in
 * the dependencies and its matchers had never been extended onto expect, so the
 * first `toHaveNoViolations` written failed with "Invalid Chai property" rather
 * than with an accessibility result. A harness that makes the accessible path
 * the awkward one gets the accessibility it deserves, and PLT-10 asks for the
 * opposite.
 */
expect.extend(axeMatchers);

/**
 * A Storage implementation for tests.
 *
 * jsdom provides localStorage, and Node 26 ships an experimental global one
 * that is unavailable unless the process was started with --localstorage-file.
 * Node's wins, so `window.localStorage` is undefined and every test touching a
 * stored preference fails with "Cannot read properties of undefined" rather
 * than with anything about storage.
 *
 * This is a harness gap rather than a defect in the code: a browser has
 * localStorage. Supplying one here keeps the environment deterministic, and the
 * behaviour that matters when storage is *unavailable* is asserted separately by
 * stubbing a throwing implementation, which is what a private window does.
 */
class MemoryStorage implements Storage {
  private entries = new Map<string, string>();

  get length(): number {
    return this.entries.size;
  }

  clear(): void {
    this.entries.clear();
  }

  getItem(key: string): string | null {
    return this.entries.get(key) ?? null;
  }

  key(index: number): string | null {
    return [...this.entries.keys()][index] ?? null;
  }

  removeItem(key: string): void {
    this.entries.delete(key);
  }

  setItem(key: string, value: string): void {
    this.entries.set(key, String(value));
  }
}

for (const target of [window, globalThis]) {
  Object.defineProperty(target, "localStorage", {
    value: new MemoryStorage(),
    writable: true,
    configurable: true,
  });
}
