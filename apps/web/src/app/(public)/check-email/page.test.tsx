import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/auth/api", () => ({ requestTokenEmail: vi.fn() }));

import { rememberSentEmail } from "@/features/auth/sentEmail";

import CheckEmailPage from "./page";

/**
 * The check-email route. What only the page does: reads which email this tab
 * last sent and survives having nothing to read.
 */

beforeEach(() => sessionStorage.clear());

describe("CheckEmailPage", () => {
  it("shows the send this tab remembers", () => {
    rememberSentEmail({ kind: "magic_link", email: "amara.eze@example.com" });
    render(<CheckEmailPage />);

    expect(screen.getByText("a•••••••e@example.com")).toBeInTheDocument();
    expect(screen.getByText("Your sign-in link")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /start again/i })).toHaveAttribute(
      "href",
      "/magic-link",
    );
  });

  it("renders with generic copy when the tab remembers nothing", () => {
    // A bookmark or a fresh tab: still a page, with the resend disabled
    // because there is no address to resend to.
    render(<CheckEmailPage />);

    expect(screen.getByText(/the address you gave/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /resend/i })).toBeDisabled();
  });
});
