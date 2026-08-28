import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { Faq } from "./Faq";
import { faq } from "./content";

/**
 * The accordion, ported from the prototype's `data-accordion-single`.
 *
 * Its behaviour is small and each part of it is a way the pattern is usually got
 * wrong: a panel with no control pointing at it, a control that does not say
 * whether it is open, several panels open at once when the design says one, and
 * a panel removed from the document while something still refers to it.
 */
describe("the FAQ", () => {
  it("opens the first answer, so the pattern is visible on arrival", () => {
    render(<Faq />);

    expect(
      screen.getByRole("button", { name: faq.items[0]!.question }),
    ).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText(faq.items[0]!.answer)).toBeVisible();
  });

  it("opens one answer at a time", async () => {
    const user = userEvent.setup();
    render(<Faq />);

    await user.click(
      screen.getByRole("button", { name: faq.items[2]!.question }),
    );

    const expanded = faq.items.filter(
      (item) =>
        screen
          .getByRole("button", { name: item.question })
          .getAttribute("aria-expanded") === "true",
    );
    expect(expanded.map((item) => item.id)).toEqual([faq.items[2]!.id]);
  });

  it("closes the open answer when its own question is used again", async () => {
    const user = userEvent.setup();
    render(<Faq />);

    const first = screen.getByRole("button", { name: faq.items[0]!.question });
    await user.click(first);

    expect(first).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByText(faq.items[0]!.answer)).not.toBeVisible();
  });

  /**
   * The panel stays in the document and is hidden with the attribute. A control
   * whose `aria-controls` points at nothing is a control assistive technology
   * cannot describe, which is what removing the panel would do.
   */
  it("keeps every panel in the document for its control to point at", () => {
    const { container } = render(<Faq />);

    for (const item of faq.items) {
      const trigger = screen.getByRole("button", { name: item.question });
      const panel = container.querySelector(
        `#${trigger.getAttribute("aria-controls")}`,
      );
      expect(panel).not.toBeNull();
      expect(panel).toHaveAttribute("aria-labelledby", trigger.id);
    }
  });

  /** Every question is a heading, so the answers can be navigated to directly. */
  it("makes every question a heading", () => {
    render(<Faq />);

    expect(screen.getAllByRole("heading", { level: 3 })).toHaveLength(
      faq.items.length,
    );
  });
});
