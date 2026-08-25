import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

let search = new URLSearchParams();
const push = vi.fn();
vi.mock("next/navigation", () => ({
  useSearchParams: () => search,
  useRouter: () => ({ push }),
}));

const consumeMagicLink = vi.fn();
const requestTokenEmail = vi.fn();
vi.mock("@/lib/auth/api", () => ({
  consumeMagicLink: (...args: unknown[]) => consumeMagicLink(...args),
  requestTokenEmail: (...args: unknown[]) => requestTokenEmail(...args),
}));

import { ApiError } from "@/lib/api/client";

import MagicLinkPage from "./page";

/**
 * The magic-link route: consuming with a token, requesting without one.
 */

beforeEach(() => {
  consumeMagicLink.mockReset();
  requestTokenEmail.mockReset();
  push.mockReset();
  sessionStorage.clear();
  search = new URLSearchParams();
});

describe("MagicLinkPage", () => {
  it("consumes an arriving token and offers the dashboard rather than yanking to it", async () => {
    search = new URLSearchParams("token=mgc_x");
    consumeMagicLink.mockResolvedValue({});
    render(<MagicLinkPage />);

    expect(await screen.findByText(/you are signed in/i)).toBeInTheDocument();
    expect(consumeMagicLink).toHaveBeenCalledWith("mgc_x");
    expect(screen.getByRole("link", { name: /go to your dashboard/i })).toHaveAttribute(
      "href",
      "/practice",
    );
  });

  it("says the link is spent, which is why forwarding the email is useless", async () => {
    search = new URLSearchParams("token=mgc_x");
    consumeMagicLink.mockResolvedValue({});
    render(<MagicLinkPage />);

    expect(await screen.findByText(/used up and will not work again/i)).toBeInTheDocument();
  });

  it("shows the request form when no token arrived", async () => {
    requestTokenEmail.mockResolvedValue(undefined);
    render(<MagicLinkPage />);

    await userEvent.type(screen.getByLabelText(/email address/i), "amara.eze@example.com");
    await userEvent.click(screen.getByRole("button", { name: /sign-in link/i }));

    await waitFor(() => expect(push).toHaveBeenCalledWith("/check-email"));
    expect(requestTokenEmail).toHaveBeenCalledWith("magic_link", "amara.eze@example.com");
  });

  it("gives an expired link its own screen with the way back", async () => {
    search = new URLSearchParams("token=mgc_x");
    consumeMagicLink.mockRejectedValue(
      new ApiError({ status: 422, code: "TOKEN_EXPIRED", message: "x" }),
    );
    render(<MagicLinkPage />);

    expect(await screen.findByRole("heading", { name: /expired/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /send a new sign-in link/i })).toHaveAttribute(
      "href",
      "/magic-link",
    );
  });
});
