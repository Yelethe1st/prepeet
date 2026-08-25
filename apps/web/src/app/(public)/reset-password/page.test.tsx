import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

let search = new URLSearchParams();
vi.mock("next/navigation", () => ({ useSearchParams: () => search }));

const confirmPasswordReset = vi.fn();
vi.mock("@/lib/auth/api", () => ({
  confirmPasswordReset: (...args: unknown[]) => confirmPasswordReset(...args),
}));

import { ApiError } from "@/lib/api/client";

import ResetPasswordPage from "./page";

/**
 * The reset route's three states: form, success, and token trouble. The page
 * owns the transitions; the form owns the fields.
 */

beforeEach(() => {
  confirmPasswordReset.mockReset();
  search = new URLSearchParams("token=rst_x");
});

async function saveNewPassword() {
  await userEvent.type(screen.getByLabelText(/^new password$/i), "a completely new passphrase");
  await userEvent.type(screen.getByLabelText(/confirm/i), "a completely new passphrase");
  await userEvent.click(screen.getByRole("button", { name: /save new password/i }));
}

describe("ResetPasswordPage", () => {
  it("resets with the token from the link and lands on success", async () => {
    confirmPasswordReset.mockResolvedValue(undefined);
    render(<ResetPasswordPage />);

    await saveNewPassword();

    await waitFor(() =>
      expect(screen.getByText(/every other device is signed out/i)).toBeInTheDocument(),
    );
    expect(confirmPasswordReset).toHaveBeenCalledWith("rst_x", "a completely new passphrase");
    // Success routes to sign-in rather than issuing a session: the reset just
    // revoked every session, including any this browser held.
    expect(screen.getByRole("link", { name: /sign in/i })).toHaveAttribute("href", "/login");
  });

  it("shows the trouble screen when the link is dead", async () => {
    confirmPasswordReset.mockRejectedValue(
      new ApiError({ status: 422, code: "TOKEN_SUPERSEDED", message: "x" }),
    );
    render(<ResetPasswordPage />);

    await saveNewPassword();

    expect(await screen.findByRole("heading", { name: /newer email/i })).toBeInTheDocument();
  });

  it("treats a missing token as an invalid link, with no form to fill", () => {
    search = new URLSearchParams();
    render(<ResetPasswordPage />);

    expect(screen.getByRole("heading", { name: /not valid/i })).toBeInTheDocument();
    expect(screen.queryByLabelText(/^new password$/i)).not.toBeInTheDocument();
  });
});
