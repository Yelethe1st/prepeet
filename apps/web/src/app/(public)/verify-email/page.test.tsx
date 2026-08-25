import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

let search = new URLSearchParams();
vi.mock("next/navigation", () => ({ useSearchParams: () => search }));

const confirmEmailVerification = vi.fn();
vi.mock("@/lib/auth/api", () => ({
  confirmEmailVerification: (...args: unknown[]) => confirmEmailVerification(...args),
}));

import { ApiError } from "@/lib/api/client";

import VerifyEmailPage from "./page";

/**
 * The verification landing. The page consumes on arrival and routes success
 * to sign-in, because verifying proves the address, not the person.
 */

beforeEach(() => {
  confirmEmailVerification.mockReset();
  search = new URLSearchParams("token=vrf_x");
});

describe("VerifyEmailPage", () => {
  it("consumes the token from the link and confirms", async () => {
    confirmEmailVerification.mockResolvedValue(undefined);
    render(<VerifyEmailPage />);

    expect(await screen.findByText(/your email address is confirmed/i)).toBeInTheDocument();
    expect(confirmEmailVerification).toHaveBeenCalledWith("vrf_x");
    // To sign-in, not a dashboard: the email could have been opened anywhere.
    expect(screen.getByRole("link", { name: /continue to sign in/i })).toHaveAttribute(
      "href",
      "/login",
    );
  });

  it("says what verification unlocked, as the prototype does", async () => {
    confirmEmailVerification.mockResolvedValue(undefined);
    render(<VerifyEmailPage />);

    expect(await screen.findByText(/screening invitations/i)).toBeInTheDocument();
  });

  it("gives an already-used link its own calm outcome", async () => {
    confirmEmailVerification.mockRejectedValue(
      new ApiError({ status: 422, code: "TOKEN_USED", message: "x" }),
    );
    render(<VerifyEmailPage />);

    expect(await screen.findByRole("heading", { name: /already been used/i })).toBeInTheDocument();
    expect(screen.getByText(/nothing further is needed/i)).toBeInTheDocument();
  });
});
