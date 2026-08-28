import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

const register = vi.fn();
vi.mock("@/lib/auth/api", () => ({
  register: (...args: unknown[]) => register(...args),
}));

import RegisterPage from "./page";

/**
 * The registration route.
 *
 * Its one decision is that success is a message rather than a navigation,
 * because registration does not sign anybody in. A page that routed to the
 * dashboard would be asserting a session that does not exist.
 */

beforeEach(() => {
  register.mockReset();
});

describe("RegisterPage", () => {
  it("renders the form", () => {
    render(<RegisterPage />);

    expect(
      screen.getByRole("heading", { name: /create/i }),
    ).toBeInTheDocument();
  });

  it("calls the real registration", async () => {
    register.mockResolvedValue(undefined);
    render(<RegisterPage />);

    await userEvent.type(
      screen.getByLabelText(/email/i),
      "daniel.okonkwo@example.com",
    );
    await userEvent.type(
      screen.getByLabelText(/^password$/i),
      "a-long-enough-password",
    );
    await userEvent.click(screen.getByRole("button", { name: /create/i }));

    expect(register).toHaveBeenCalledWith({
      email: "daniel.okonkwo@example.com",
      password: "a-long-enough-password",
      account_type: "candidate",
    });
  });

  it("stays here and says what happens next, rather than pretending to sign in", async () => {
    register.mockResolvedValue(undefined);
    render(<RegisterPage />);

    await userEvent.type(
      screen.getByLabelText(/email/i),
      "daniel.okonkwo@example.com",
    );
    await userEvent.type(
      screen.getByLabelText(/^password$/i),
      "a-long-enough-password",
    );
    await userEvent.click(screen.getByRole("button", { name: /create/i }));

    expect(await screen.findByRole("status")).toHaveTextContent(
      /check your email/i,
    );
  });

  it("offers a way back to signing in", () => {
    render(<RegisterPage />);

    expect(screen.getByRole("link", { name: /sign in/i })).toHaveAttribute(
      "href",
      "/login",
    );
  });

  it("has no accessibility violations", async () => {
    const { container } = render(<RegisterPage />);

    expect(await axe(container)).toHaveNoViolations();
  });

  it("has exactly one first-level heading", () => {
    render(<RegisterPage />);

    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
  });
});
