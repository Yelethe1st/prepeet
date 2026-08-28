import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  DEFAULT_THEME,
  STORAGE_KEY,
  applyTheme,
  resolveTheme,
  setTheme,
  themeScript,
} from "./themePreference";

/**
 * Theme resolution.
 *
 * The rule is short and every part of it was chosen: an explicit choice the
 * person made wins, and otherwise the product is dark. What is asserted here is
 * mostly the failure modes, because the happy path is one line and the ways it
 * goes wrong are not.
 */

beforeEach(() => {
  // Un-stub first. Several tests replace localStorage with one that throws, and
  // clearing before restoring would call clear() on the replacement, which
  // fails for a reason that has nothing to do with the test being run.
  vi.unstubAllGlobals();
  document.documentElement.removeAttribute("data-theme");
  window.localStorage.clear();
});

describe("resolveTheme", () => {
  it("is dark when nothing has been chosen", () => {
    expect(resolveTheme()).toBe("dark");
  });

  it("agrees with the exported default, so the two cannot drift", () => {
    expect(resolveTheme()).toBe(DEFAULT_THEME);
  });

  it.each(["light", "dark"] as const)(
    "honours an explicit %s choice",
    (theme) => {
      window.localStorage.setItem(STORAGE_KEY, theme);

      expect(resolveTheme()).toBe(theme);
    },
  );

  /**
   * A stored value that is neither theme is not a reason to render unstyled. It
   * happens when a key is edited by hand, shared with another product on the
   * same origin, or left behind by an older version.
   */
  it("falls back when the stored value is not a theme", () => {
    window.localStorage.setItem(STORAGE_KEY, "solarized");

    expect(resolveTheme()).toBe("dark");
  });

  /**
   * localStorage throws rather than returning null in a private window in some
   * browsers, and wherever a person has blocked site data. A theme module that
   * threw would take the whole page with it, before anything had rendered.
   */
  it("survives storage being unavailable", () => {
    vi.stubGlobal("localStorage", {
      getItem() {
        throw new Error("The operation is insecure.");
      },
      setItem() {
        throw new Error("The operation is insecure.");
      },
    });

    expect(() => resolveTheme()).not.toThrow();
    expect(resolveTheme()).toBe("dark");
  });

  /**
   * The system preference is deliberately not consulted. Asserted so that it
   * reads as a decision rather than as something nobody thought about, and so
   * reversing it is a deliberate change with a failing test to notice.
   */
  it("does not follow the operating system's preference", () => {
    vi.stubGlobal("matchMedia", () => ({
      matches: true,
      media: "(prefers-color-scheme: light)",
    }));

    expect(resolveTheme()).toBe("dark");
  });
});

describe("setTheme", () => {
  it("records the choice and applies it", () => {
    setTheme("light");

    expect(window.localStorage.getItem(STORAGE_KEY)).toBe("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("still applies the theme when storage refuses to keep it", () => {
    vi.stubGlobal("localStorage", {
      getItem: () => null,
      setItem() {
        throw new Error("QuotaExceededError");
      },
    });

    setTheme("light");

    // The choice does not survive a reload, which is a limitation. A toggle
    // that appears to do nothing is a worse one.
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });
});

describe("applyTheme", () => {
  it("sets the attribute the stylesheets key off", () => {
    applyTheme("dark");

    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });
});

describe("themeScript", () => {
  /**
   * This runs in the document head before anything paints, and exists only to
   * avoid a flash of the wrong theme. It is inlined as a string, so none of the
   * usual guarantees apply to it and these are the only checks it gets.
   */
  it("names the same key and default the module uses", () => {
    expect(themeScript).toContain(STORAGE_KEY);
    expect(themeScript).toContain(DEFAULT_THEME);
  });

  it("cannot break the document if storage throws", () => {
    expect(themeScript).toContain("try");
    expect(themeScript).toContain("catch");
  });

  it("closes no tag that would end the script early", () => {
    // A literal closing script tag inside an inlined string ends the block, and
    // everything after it becomes markup: it renders as text on the page and
    // the theme never applies.
    expect(themeScript).not.toContain("</script");
  });

  it("actually sets the theme when run", () => {
    new Function(themeScript)();

    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("honours a stored choice when run", () => {
    window.localStorage.setItem(STORAGE_KEY, "light");

    new Function(themeScript)();

    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  /**
   * The inlined script duplicates the module's logic, because it must run as a
   * string with no module loading. This is what holds the copy to the original.
   */
  it("agrees with resolveTheme in both cases", () => {
    new Function(themeScript)();
    expect(document.documentElement.getAttribute("data-theme")).toBe(
      resolveTheme(),
    );

    window.localStorage.setItem(STORAGE_KEY, "light");
    new Function(themeScript)();
    expect(document.documentElement.getAttribute("data-theme")).toBe(
      resolveTheme(),
    );
  });
});
