import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { SiteHeader } from "./SiteHeader";

/**
 * The header, which is the only part of this page with behaviour.
 *
 * The prototype does all of this in a script at the bottom of index.html, and
 * every one of these was a line in that script rather than something the markup
 * expressed. They are properties of the component here, which is why they can be
 * tested at all.
 */
describe("the site header", () => {
  it("hides the menu until it is asked for", () => {
    render(<SiteHeader />);

    const button = screen.getByRole("button", { name: "Open navigation menu" });
    expect(button).toHaveAttribute("aria-expanded", "false");
    expect(document.getElementById("marketing-menu")).toHaveAttribute("hidden");
  });

  /**
   * A button labelled "open" that closes the thing is worse than an unlabelled
   * one, because it is confidently wrong.
   */
  it("says whether the menu is open, in the name and in the state", async () => {
    const user = userEvent.setup();
    render(<SiteHeader />);

    await user.click(
      screen.getByRole("button", { name: "Open navigation menu" }),
    );

    const button = screen.getByRole("button", {
      name: "Close navigation menu",
    });
    expect(button).toHaveAttribute("aria-expanded", "true");
    expect(document.getElementById("marketing-menu")).not.toHaveAttribute(
      "hidden",
    );

    await user.click(button);
    expect(
      screen.getByRole("button", { name: "Open navigation menu" }),
    ).toHaveAttribute("aria-expanded", "false");
  });

  /**
   * Every link in the menu is an anchor to a section of this same page, so
   * leaving it open would scroll the page behind a panel covering it.
   */
  it("closes the menu when a link in it is followed", async () => {
    const user = userEvent.setup();
    render(<SiteHeader />);

    await user.click(
      screen.getByRole("button", { name: "Open navigation menu" }),
    );
    const mobile = screen.getByRole("navigation", { name: "Mobile" });
    await user.click(
      within(mobile).getByRole("link", { name: "How it works" }),
    );

    expect(document.getElementById("marketing-menu")).toHaveAttribute("hidden");
  });

  /**
   * Escape closes it and focus comes back to the button that opened it.
   * Without the second half, dismissing the menu from a keyboard leaves focus
   * on an element that is now hidden, and the next Tab starts from nowhere.
   */
  it("closes on Escape and returns focus to the button", async () => {
    const user = userEvent.setup();
    render(<SiteHeader />);

    const button = screen.getByRole("button", { name: "Open navigation menu" });
    await user.click(button);
    await user.keyboard("{Escape}");

    expect(document.getElementById("marketing-menu")).toHaveAttribute("hidden");
    expect(
      screen.getByRole("button", { name: "Open navigation menu" }),
    ).toHaveFocus();
  });

  /** A key that is not Escape is not a dismissal. */
  it("ignores other keys while the menu is open", async () => {
    const user = userEvent.setup();
    render(<SiteHeader />);

    await user.click(
      screen.getByRole("button", { name: "Open navigation menu" }),
    );
    await user.keyboard("{ArrowDown}");

    expect(document.getElementById("marketing-menu")).not.toHaveAttribute(
      "hidden",
    );
  });

  it("offers signing in and getting started", () => {
    render(<SiteHeader />);

    expect(screen.getAllByRole("link", { name: "Sign in" })[0]).toHaveAttribute(
      "href",
      "/login",
    );
    expect(screen.getByRole("link", { name: "Get started" })).toHaveAttribute(
      "href",
      "/register",
    );
  });
});
