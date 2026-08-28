import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

import { Button } from "./Button";

describe("Button", () => {
  it("defaults to type button, so it cannot submit a form by accident", () => {
    render(<Button>Continue</Button>);

    expect(screen.getByRole("button", { name: "Continue" })).toHaveAttribute(
      "type",
      "button",
    );
  });

  /**
   * Variants and sizes render differently, without this asserting which
   * utilities they use.
   *
   * Naming the classes would make the test fail on a restyle and pass on a
   * broken component, which is backwards. What matters is that the mapping
   * exists and is distinct: a variant that rendered identically to another is a
   * variant that does nothing. How it looks is settled by the browser suite,
   * which compares against a screenshot and checks contrast as rendered.
   */
  it("renders each variant differently", () => {
    const seen = new Set<string>();

    for (const variant of [
      "primary",
      "secondary",
      "ghost",
      "danger",
    ] as const) {
      const { unmount } = render(<Button variant={variant}>Continue</Button>);
      seen.add(screen.getByRole("button").className);
      unmount();
    }

    expect(seen.size).toBe(4);
  });

  it("renders each size differently", () => {
    const seen = new Set<string>();

    for (const size of ["sm", "md", "lg"] as const) {
      const { unmount } = render(<Button size={size}>Continue</Button>);
      seen.add(screen.getByRole("button").className);
      unmount();
    }

    expect(seen.size).toBe(3);
  });

  it("fills its container only when asked to", () => {
    const { unmount } = render(<Button block>Continue</Button>);
    const blocked = screen.getByRole("button").className;
    unmount();

    render(<Button>Continue</Button>);

    expect(blocked).not.toBe(screen.getByRole("button").className);
  });

  /**
   * A form that can be submitted twice will be. For a login that is harmless;
   * for starting an interview it is a second billed session, so the behaviour
   * belongs in the component rather than in each form that remembers it.
   */
  it("refuses a second click while busy", async () => {
    const onClick = vi.fn();
    render(
      <Button busy onClick={onClick}>
        Sign in
      </Button>,
    );

    await userEvent.click(screen.getByRole("button"));

    expect(onClick).not.toHaveBeenCalled();
  });

  it("announces that it is busy rather than only looking it", () => {
    render(<Button busy>Sign in</Button>);

    expect(screen.getByRole("button")).toHaveAttribute("aria-busy", "true");
  });

  it("says what it is doing when given a busy label", () => {
    render(
      <Button busy busyLabel="Signing in…">
        Sign in
      </Button>,
    );

    expect(
      screen.getByRole("button", { name: "Signing in…" }),
    ).toBeInTheDocument();
  });

  it("is not busy when it is merely disabled", () => {
    render(<Button disabled>Sign in</Button>);

    expect(screen.getByRole("button")).not.toHaveAttribute("aria-busy");
  });

  it("still calls its handler when neither busy nor disabled", async () => {
    const onClick = vi.fn();
    render(<Button onClick={onClick}>Sign in</Button>);

    await userEvent.click(screen.getByRole("button"));

    expect(onClick).toHaveBeenCalledOnce();
  });

  it("has no accessibility violations", async () => {
    const { container } = render(
      <Button busy busyLabel="Signing in…">
        Sign in
      </Button>,
    );

    expect(await axe(container)).toHaveNoViolations();
  });
});
