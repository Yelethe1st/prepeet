import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { STORAGE_KEY } from "../themePreference";
import { ThemeToggle } from "./ThemeToggle";

describe("the theme toggle", () => {
  beforeEach(() => {
    window.localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
  });

  /**
   * The product defaults to dark, so the button on arrival offers the other one.
   * It says which, rather than reporting itself as pressed: the prototype's
   * `aria-pressed` on an unlabelled icon button announces "button, pressed" and
   * nothing about what pressing it would do.
   */
  it("offers the theme that is not currently on", () => {
    render(<ThemeToggle />);

    expect(
      screen.getByRole("button", { name: "Switch to the light theme" }),
    ).toBeInTheDocument();
  });

  it("applies and remembers the choice", async () => {
    const user = userEvent.setup();
    render(<ThemeToggle />);

    await user.click(
      screen.getByRole("button", { name: "Switch to the light theme" }),
    );

    expect(document.documentElement).toHaveAttribute("data-theme", "light");
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe("light");
    expect(
      screen.getByRole("button", { name: "Switch to the dark theme" }),
    ).toBeInTheDocument();
  });

  /**
   * The stored choice is read through the external store, so a preference set
   * before this render is the one shown, without a second render to correct it.
   */
  it("shows a choice made before this render", () => {
    window.localStorage.setItem(STORAGE_KEY, "light");

    render(<ThemeToggle />);

    expect(
      screen.getByRole("button", { name: "Switch to the dark theme" }),
    ).toBeInTheDocument();
  });

  /**
   * The prototype puts a toggle in the header and another in the footer. Two
   * copies of the control reading their own state is how one of them goes on
   * offering the theme that is already on.
   */
  it("keeps two copies of itself in agreement", async () => {
    const user = userEvent.setup();
    render(
      <>
        <ThemeToggle />
        <ThemeToggle withLabel />
      </>,
    );

    await user.click(
      screen.getAllByRole("button", { name: "Switch to the light theme" })[0]!,
    );

    expect(
      screen.getAllByRole("button", { name: "Switch to the dark theme" }),
    ).toHaveLength(2);
  });

  /**
   * Another tab is the other thing that changes it, and the browser reports
   * that as a storage event rather than through anything this tab called.
   */
  it("follows a change made in another tab", () => {
    render(<ThemeToggle />);

    act(() => {
      window.localStorage.setItem(STORAGE_KEY, "light");
      window.dispatchEvent(
        new StorageEvent("storage", { key: STORAGE_KEY, newValue: "light" }),
      );
    });

    expect(
      screen.getByRole("button", { name: "Switch to the dark theme" }),
    ).toBeInTheDocument();
  });

  /** The footer's copy of it shows the label rather than hiding it in a name. */
  it("can show its label", () => {
    render(<ThemeToggle withLabel />);

    const button = screen.getByRole("button", {
      name: "Switch to the light theme",
    });
    expect(button).toHaveTextContent("Switch to the light theme");
    expect(button).not.toHaveAttribute("aria-label");
  });
});
