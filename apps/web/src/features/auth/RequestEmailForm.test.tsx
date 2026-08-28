import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

import { ApiError } from "@/lib/api/client";

import { RequestEmailForm } from "./RequestEmailForm";

/**
 * The request half of every token flow.
 *
 * What matters here is what the form does with the answers: success and
 * cooldown both move forward, because in both cases an email exists to check,
 * and only a genuine failure keeps the person on the form.
 */

describe("RequestEmailForm", () => {
  it("requests the flow's email and reports the address it used", async () => {
    const request = vi.fn().mockResolvedValue(undefined);
    const onSent = vi.fn();
    render(
      <RequestEmailForm
        kind="password_reset"
        request={request}
        onSent={onSent}
      />,
    );

    await userEvent.type(
      screen.getByLabelText(/email address/i),
      "Amara.Eze@Example.com",
    );
    await userEvent.click(
      screen.getByRole("button", { name: /email me a reset link/i }),
    );

    // Normalised by the schema before the network sees it, so the server and
    // the check-email screen agree about which address it was.
    expect(request).toHaveBeenCalledWith(
      "password_reset",
      "amara.eze@example.com",
    );
    expect(onSent).toHaveBeenCalledWith("amara.eze@example.com");
  });

  it("treats a cooldown as sent, because the recent email is the one to check", async () => {
    const request = vi.fn().mockRejectedValue(
      new ApiError({
        status: 429,
        code: "RESEND_COOLING_DOWN",
        message: "wait",
      }),
    );
    const onSent = vi.fn();
    render(
      <RequestEmailForm kind="magic_link" request={request} onSent={onSent} />,
    );

    await userEvent.type(screen.getByLabelText(/email address/i), "a@b.co");
    await userEvent.click(
      screen.getByRole("button", { name: /sign-in link/i }),
    );

    expect(onSent).toHaveBeenCalledWith("a@b.co");
  });

  it("keeps a real failure on the form, with the reference support would ask for", async () => {
    const request = vi.fn().mockRejectedValue(
      new ApiError({
        status: 500,
        code: "INTERNAL",
        message: "Something went wrong.",
        requestId: "req_9",
      }),
    );
    const onSent = vi.fn();
    render(<RequestEmailForm kind="otp" request={request} onSent={onSent} />);

    await userEvent.type(screen.getByLabelText(/email address/i), "a@b.co");
    await userEvent.click(
      screen.getByRole("button", { name: /email me a code/i }),
    );

    expect(onSent).not.toHaveBeenCalled();
    expect(screen.getByText(/something went wrong/i)).toBeInTheDocument();
    expect(screen.getByText(/req_9/)).toBeInTheDocument();
  });

  it("refuses a non-address before any request is made", async () => {
    const request = vi.fn();
    render(
      <RequestEmailForm
        kind="password_reset"
        request={request}
        onSent={vi.fn()}
      />,
    );

    await userEvent.type(
      screen.getByLabelText(/email address/i),
      "not-an-address",
    );
    await userEvent.click(screen.getByRole("button", { name: /reset link/i }));

    expect(request).not.toHaveBeenCalled();
    expect(screen.getByText(/valid email address/i)).toBeInTheDocument();
  });

  it("states the flow's expiry, which the email will repeat", () => {
    render(
      <RequestEmailForm
        kind="password_reset"
        request={vi.fn()}
        onSent={vi.fn()}
      />,
    );
    expect(screen.getByText(/30 minutes/)).toBeInTheDocument();
  });

  it("has no accessibility violations", async () => {
    const { container } = render(
      <RequestEmailForm
        kind="password_reset"
        request={vi.fn()}
        onSent={vi.fn()}
      />,
    );
    expect(await axe(container)).toHaveNoViolations();
  });
});
