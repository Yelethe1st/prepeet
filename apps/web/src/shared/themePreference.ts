/**
 * Which theme the product renders in.
 *
 * Ported from screens/assets/js/theme.js, with one deliberate difference: the
 * prototype declares a per-page default and uses light on almost every screen,
 * and the product is dark. That is a product decision rather than a port, so it
 * is stated here rather than left as a value somebody finds in a layout.
 */

/** The themes the stylesheets define. Both are complete; neither is a variant. */
export type Theme = "light" | "dark";

/**
 * The theme when nobody has chosen one.
 *
 * Dark. An interview is a long, concentrated session that many people take in
 * the evening, and a bright page for forty minutes is a real cost rather than a
 * matter of taste.
 */
export const DEFAULT_THEME: Theme = "dark";

/** Where an explicit choice is kept. The prototype's key, so a browser that has
 * used the prototype keeps its preference. */
export const STORAGE_KEY = "prepeet.theme";

/** The attribute the stylesheets key off. */
const ATTRIBUTE = "data-theme";

function isTheme(value: unknown): value is Theme {
  return value === "light" || value === "dark";
}

/**
 * stored returns an explicit choice, or null.
 *
 * localStorage throws rather than returning null in a private window in some
 * browsers, and wherever a person has blocked site data. A theme module that
 * threw would take the whole page down before anything had rendered, over a
 * preference.
 */
function stored(): Theme | null {
  try {
    const value = window.localStorage.getItem(STORAGE_KEY);
    return isTheme(value) ? value : null;
  } catch {
    return null;
  }
}

/**
 * resolveTheme decides which theme to render.
 *
 * An explicit choice wins; otherwise the default. The operating system's
 * preference is deliberately not consulted: most systems report light, so
 * following it would mean the product's default was effectively light while
 * claiming otherwise. Somebody who wants light chooses it, and the choice
 * sticks.
 */
export function resolveTheme(): Theme {
  return stored() ?? DEFAULT_THEME;
}

/** applyTheme puts the theme on the document. */
export function applyTheme(theme: Theme): void {
  document.documentElement.setAttribute(ATTRIBUTE, theme);
}

/** Everything currently rendering the theme, so a change reaches all of it. */
const listeners = new Set<() => void>();

/**
 * subscribeToTheme reports a change to whoever is displaying it.
 *
 * The theme lives on the document and in storage, neither of which React can
 * see, so a component that shows it has to be told. Two things can change it:
 * this tab, through setTheme, and another tab, which the browser reports as a
 * storage event.
 *
 * It exists because the alternative is reading the stored value into state in
 * an effect, which is a render, then a second render, on every mount. The React
 * compiler rejects that pattern outright, and it is also how two toggles on the
 * same page end up disagreeing: the footer's copy would never hear about the
 * header's click.
 */
export function subscribeToTheme(listener: () => void): () => void {
  listeners.add(listener);
  window.addEventListener("storage", listener);

  return () => {
    listeners.delete(listener);
    window.removeEventListener("storage", listener);
  };
}

/**
 * setTheme records a choice and applies it.
 *
 * Applied even when it cannot be stored. Storage refuses in a private window
 * and when a quota is exhausted, and a toggle that appears to do nothing is a
 * worse failure than a preference that does not survive a reload.
 */
export function setTheme(theme: Theme): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, theme);
  } catch {
    // Deliberately ignored. See above.
  }
  applyTheme(theme);

  for (const listener of listeners) listener();
}

/**
 * themeScript runs in the document head, before anything paints.
 *
 * It exists because the alternative is a flash of the wrong theme: React sets
 * the attribute after hydration, and the page is visible well before that. A
 * white flash before a dark page is unpleasant, and a dark flash before a light
 * one is worse for anybody who chose light because bright is painful.
 *
 * It duplicates the logic above rather than importing it, because it must run
 * as a string in the head with no module loading. The duplication is small,
 * deliberate, and held to the module by tests that run this string and compare
 * the result.
 */
export const themeScript = `
try {
  var stored = window.localStorage.getItem(${JSON.stringify(STORAGE_KEY)});
} catch (e) {
  var stored = null;
}
document.documentElement.setAttribute(
  ${JSON.stringify(ATTRIBUTE)},
  stored === "light" || stored === "dark" ? stored : ${JSON.stringify(DEFAULT_THEME)}
);
`;
