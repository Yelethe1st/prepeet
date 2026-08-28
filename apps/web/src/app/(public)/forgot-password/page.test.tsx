import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));

const requestTokenEmail = vi.fn();
vi.mock("@/lib/auth/api", () => ({
  requestTokenEmail: (...args: unknown[]) => requestTokenEmail(...args),
}));

import { readSentEmail } from "@/features/auth/sentEmail";

import ForgotPasswordPage from "./page";

/**
 * The recovery request route. What only the page does: wires the reset flow
 * into the shared form, remembers what was sent for the next screen, and goes
 * there.
 */

beforeEach(() => {
  push.mockReset();
  requestTokenEmail.mockReset();
  sessionStorage.clear();
});

describe("ForgotPasswordPage", () => {
  it("requests a reset email, remembers it, and moves to check-email", async () => {
    requestTokenEmail.mockResolvedValue(undefined);
    render(<ForgotPasswordPage />);

    await userEvent.type(
      screen.getByLabelText(/email address/i),
      "amara.eze@example.com",
    );
    await userEvent.click(screen.getByRole("button", { name: /reset link/i }));

    await waitFor(() => expect(push).toHaveBeenCalledWith("/check-email"));
    expect(requestTokenEmail).toHaveBeenCalledWith(
      "password_reset",
      "amara.eze@example.com",
    );
    // The next screen reads this; the URL deliberately carries nothing.
    expect(readSentEmail()).toEqual({
      kind: "password_reset",
      email: "amara.eze@example.com",
    });
  });
});
