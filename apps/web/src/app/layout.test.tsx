import { describe, expect, it } from "vitest";

import { DEFAULT_THEME, themeScript } from "@/shared/themePreference";

/**
 * The document shell.
 *
 * Not rendered, because a root layout returns html and body and React Testing
 * Library cannot mount those into a document that already has them. What is
 * asserted instead is the pair of properties the layout exists to provide, both
 * of which are about what reaches the browser before any React runs.
 */
describe("the document shell", () => {
  it("carries the default theme in the markup the server sends", async () => {
    const layout = await import("./layout");
    const rendered = layout.default({ children: null });

    expect(rendered.props["data-theme"]).toBe(DEFAULT_THEME);
    expect(rendered.props.lang).toBe("en-GB");
  });

  /**
   * The script changes the attribute React rendered, which is its job. Without
   * this React reports the difference as a hydration mismatch and, in
   * development, logs an error on every load.
   */
  it("tells React the theme attribute will be corrected", async () => {
    const layout = await import("./layout");
    const rendered = layout.default({ children: null });

    expect(rendered.props.suppressHydrationWarning).toBe(true);
  });

  /**
   * The inline script is what avoids a flash of the wrong theme for anybody who
   * chose light. A layout that rendered the attribute and not the script would
   * look correct for the default and flash for everybody else.
   */
  it("inlines the theme script", async () => {
    const layout = await import("./layout");
    const rendered = layout.default({ children: null });

    const html = JSON.stringify(rendered);
    expect(html).toContain("prepeet.theme");
    expect(themeScript).toContain("prepeet.theme");
  });
});
