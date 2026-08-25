import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));

const consumeOtp = vi.fn();
const requestTokenEmail = vi.fn();
vi.mock("@/lib/auth/api", () => ({
  consumeOtp: (...args: unknown[]) => consumeOtp(...args),
  requestTokenEmail: (...args: unknown[]) => requestTokenEmail(...args),
}));

import { ApiError } from "@/lib/api/client";
import { rememberSentEmail } from "@/features/auth/sentEmail";

import OtpPage from "./page";

/**
 * The one-time-code route: request, entry and trouble, decided by what this
 * tab remembers sending.
 */

beforeEach(() => {
  push.mockReset();
  consumeOtp.mockReset();
  requestTokenEmail.mockReset();
  sessionStorage.clear();
});

describe("OtpPage", () => {
  it("asks for the address first when this tab sent nothing", () => {
    render(<OtpPage />);
    expect(screen.getByLabelText(/email address/i)).toBeInTheDocument();
  });

  it("moves to code entry after the request, without leaving the page", async () => {
    requestTokenEmail.mockResolvedValue(undefined);
    render(<OtpPage />);

    await userEvent.type(screen.getByLabelText(/email address/i), "amara.eze@example.com");
    await userEvent.click(screen.getByRole("button", { name: /email me a code/i }));

    expect(await screen.findByLabelText(/six-digit code/i)).toBeInTheDocument();
  });

  it("goes straight to code entry when this tab already asked", () => {
    rememberSentEmail({ kind: "otp", email: "amara.eze@example.com" });
    render(<OtpPage />);

    expect(screen.getByLabelText(/six-digit code/i)).toBeInTheDocument();
  });

  it("signs in and navigates on the right code", async () => {
    rememberSentEmail({ kind: "otp", email: "amara.eze@example.com" });
    consumeOtp.mockResolvedValue({});
    render(<OtpPage />);

    await userEvent.type(screen.getByLabelText(/six-digit code/i), "123456");
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));

    await waitFor(() => expect(push).toHaveBeenCalledWith("/practice"));
  });

  it("shows the exhausted screen once the code is guessed dead", async () => {
    rememberSentEmail({ kind: "otp", email: "amara.eze@example.com" });
    consumeOtp.mockRejectedValue(
      new ApiError({ status: 422, code: "CODE_ATTEMPTS_EXHAUSTED", message: "x" }),
    );
    render(<OtpPage />);

    await userEvent.type(screen.getByLabelText(/six-digit code/i), "123456");
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));

    expect(
      await screen.findByRole("heading", { name: /too many incorrect codes/i }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /send a new code/i })).toHaveAttribute("href", "/otp");
  });
});
