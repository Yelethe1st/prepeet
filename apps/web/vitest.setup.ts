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

/**
 * Browser APIs jsdom does not implement, which Radix needs.
 *
 * Radix builds its own listbox rather than using a native select, which is what
 * buys focus management, typeahead and correct assistive-technology wiring. That
 * implementation uses pointer capture, scrolls the highlighted option into view,
 * and observes its own size, none of which exists in jsdom.
 *
 * Without these a Radix dropdown never opens in a test and every assertion about
 * its contents fails with "unable to find role option", which reads as a broken
 * component rather than a missing environment.
 *
 * These are stubs and not implementations. What they enable is testing what the
 * component does; how it is positioned and scrolled is a rendering question, and
 * the browser suite is where that is answered.
 */
if (!Element.prototype.hasPointerCapture) {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
}

if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {};
}

if (!("ResizeObserver" in globalThis)) {
  class ResizeObserverStub {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  Object.defineProperty(globalThis, "ResizeObserver", {
    value: ResizeObserverStub,
    writable: true,
    configurable: true,
  });
}
