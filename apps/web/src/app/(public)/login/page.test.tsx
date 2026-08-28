import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));

const signIn = vi.fn();
vi.mock("@/lib/auth/api", () => ({
  signIn: (...args: unknown[]) => signIn(...args),
}));

import LoginPage from "./page";

/**
 * The sign-in route.
 *
 * The form has its own tests; what is left here is what only the page does, and
 * it is not nothing: it decides where somebody goes after signing in, and it
 * decides which call the form is given. Both were untested and invisible,
 * because this file was excluded from coverage as a composition point with no
 * logic. It has logic.
 */

beforeEach(() => {
  push.mockReset();
  signIn.mockReset();
});

async function signInWith(
  email = "daniel.okonkwo@example.com",
  password = "a-long-password",
) {
  await userEvent.type(screen.getByLabelText(/email/i), email);
  await userEvent.type(screen.getByLabelText(/^password$/i), password);
  await userEvent.click(screen.getByRole("button", { name: /sign in/i }));
}

describe("LoginPage", () => {
  it("renders the form", () => {
    render(<LoginPage />);

    expect(
      screen.getByRole("heading", { name: /sign in/i }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
  });

  it("calls the real sign-in with what was typed", async () => {
    signIn.mockResolvedValue({});
    render(<LoginPage />);

    await signInWith();

    expect(signIn).toHaveBeenCalledWith({
      email: "daniel.okonkwo@example.com",
      password: "a-long-password",
    });
  });

  it("navigates once there is a session", async () => {
    signIn.mockResolvedValue({});
    render(<LoginPage />);

    await signInWith();

    await waitFor(() => expect(push).toHaveBeenCalledOnce());
  });

  /**
   * The failure that matters most here. Navigating on a failed sign-in would
   * land somebody on a page that immediately bounces them back, and the reason
   * would never be shown.
   */
  it("does not navigate when signing in failed", async () => {
    signIn.mockRejectedValue(new Error("nope"));
    render(<LoginPage />);

    await signInWith();

    await screen.findByRole("alert");
    expect(push).not.toHaveBeenCalled();
  });

  it("offers a way to register, so a new person is not stuck here", () => {
    render(<LoginPage />);

    expect(
      screen.getByRole("link", { name: /create an account/i }),
    ).toHaveAttribute("href", "/register");
  });

  it("has no accessibility violations", async () => {
    const { container } = render(<LoginPage />);

    expect(await axe(container)).toHaveNoViolations();
  });

  it("has exactly one first-level heading", () => {
    render(<LoginPage />);

    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
  });
});
