import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

import { Button } from "./Button";

describe("Button", () => {
  it("defaults to type button, so it cannot submit a form by accident", () => {
    render(<Button>Continue</Button>);

    expect(screen.getByRole("button", { name: "Continue" })).toHaveAttribute("type", "button");
  });

  it("emits the prototype's variant classes", () => {
    render(
      <Button variant="ghost" size="lg" block>
        Continue
      </Button>,
    );

    const button = screen.getByRole("button");
    expect(button).toHaveClass("btn", "btn-ghost", "btn-lg", "btn-block");
  });

  it("does not emit a size class for the default size, as the prototype does not", () => {
    render(<Button>Continue</Button>);

    expect(screen.getByRole("button").className).not.toContain("btn-md");
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

    expect(screen.getByRole("button", { name: "Signing in…" })).toBeInTheDocument();
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
