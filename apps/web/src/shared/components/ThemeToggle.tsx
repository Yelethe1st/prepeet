"use client";

import { Moon, Sun } from "lucide-react";
import { useSyncExternalStore } from "react";

import {
  DEFAULT_THEME,
  resolveTheme,
  setTheme,
  subscribeToTheme,
  type Theme,
} from "../themePreference";
import { Icon } from "./Icon";

/**
 * The control that changes the theme.
 *
 * `themePreference` has existed since WEB-01 with nothing calling `setTheme`:
 * the module decided the theme and the head script applied it, but a person had
 * no way to choose. The marketing page is the first screen the prototype gives
 * a toggle, so this is the first screen that needed one.
 *
 * The current theme is read through `useSyncExternalStore`, which is the hook
 * for exactly this shape: a value that lives outside React, in storage and on
 * the document element, and that the server cannot see. It takes a separate
 * server snapshot, so the markup React renders on the server carries the default
 * and the browser corrects it in the same pass rather than in a second render.
 *
 * Reading it into state in an effect was the first attempt and is wrong twice
 * over: it renders, then renders again, on every mount, and it makes each toggle
 * its own copy of the answer, so the footer's would go on offering light after
 * the header's had already switched to it.
 *
 * One deviation from the prototype, recorded: its header toggle is an icon
 * button with `aria-pressed` and no text, so assistive technology announces
 * "button, pressed" and nothing about what it does. It gets a name here.
 * Pressed is also the wrong state for it: this is not a thing that is on or
 * off, it is one that switches to the other theme, so it says which.
 */
export function ThemeToggle({ withLabel = false }: { withLabel?: boolean }) {
  const theme = useSyncExternalStore<Theme>(
    subscribeToTheme,
    resolveTheme,
    () => DEFAULT_THEME,
  );

  const next: Theme = theme === "dark" ? "light" : "dark";
  const label =
    next === "light" ? "Switch to the light theme" : "Switch to the dark theme";

  return (
    <button
      type="button"
      aria-label={withLabel ? undefined : label}
      onClick={() => setTheme(next)}
      className={
        "inline-flex items-center gap-2 rounded-md border border-border-strong bg-surface " +
        "px-3 text-fg-2 transition-colors hover:bg-surface-3 hover:text-fg " +
        (withLabel ? "min-h-8 text-xs" : "min-h-9 w-9 justify-center px-0")
      }
    >
      <Icon glyph={next === "light" ? Sun : Moon} size="sm" />
      {withLabel ? label : null}
    </button>
  );
}
