import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

import { ApiError } from "@/lib/api/client";

import { OtpForm } from "./OtpForm";

function renderForm(overrides: Partial<Parameters<typeof OtpForm>[0]> = {}) {
  const props = {
    email: "amara.eze@example.com",
    consume: vi.fn().mockResolvedValue(undefined),
    onSignedIn: vi.fn(),
    onTokenTrouble: vi.fn(),
    ...overrides,
  };
  render(<OtpForm {...props} />);
  return props;
}

describe("OtpForm", () => {
  it("signs in with the code and the address it went to", async () => {
    const props = renderForm();

    await userEvent.type(screen.getByLabelText(/six-digit code/i), "123456");
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));

    expect(props.consume).toHaveBeenCalledWith(
      "amara.eze@example.com",
      "123456",
    );
    expect(props.onSignedIn).toHaveBeenCalled();
  });

  it("masks the address it names", () => {
    renderForm();
    expect(screen.getByText("a•••••••e@example.com")).toBeInTheDocument();
  });

  it("asks the phone for the code from the email", () => {
    renderForm();
    // one-time-code is what lets a mail app offer the digits above the
    // keyboard; without it the person transcribes by hand.
    expect(screen.getByLabelText(/six-digit code/i)).toHaveAttribute(
      "autocomplete",
      "one-time-code",
    );
  });

  it("refuses a non-code before the network sees it", async () => {
    const props = renderForm();

    await userEvent.type(screen.getByLabelText(/six-digit code/i), "12345");
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));

    expect(props.consume).not.toHaveBeenCalled();
    expect(screen.getByText(/six digits from the email/i)).toBeInTheDocument();
  });

  it("keeps a wrong code inline with the supersession rule spelled out", async () => {
    renderForm({
      consume: vi
        .fn()
        .mockRejectedValue(
          new ApiError({ status: 422, code: "CODE_INCORRECT", message: "x" }),
        ),
    });

    await userEvent.type(screen.getByLabelText(/six-digit code/i), "123456");
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));

    expect(screen.getByText(/older codes stop working/i)).toBeInTheDocument();
  });

  it.each([
    ["CODE_ATTEMPTS_EXHAUSTED", "CODE_ATTEMPTS_EXHAUSTED"],
    ["TOKEN_EXPIRED", "TOKEN_EXPIRED"],
  ])("hands %s to the trouble screen", async (code, expected) => {
    const props = renderForm({
      consume: vi
        .fn()
        .mockRejectedValue(new ApiError({ status: 422, code, message: "x" })),
    });

    await userEvent.type(screen.getByLabelText(/six-digit code/i), "123456");
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));

    expect(props.onTokenTrouble).toHaveBeenCalledWith(expected);
  });

  it("has no accessibility violations", async () => {
    renderForm();
    // The region rule is off because this renders the form alone; on the
    // page, AuthShell's <main> is the landmark it complains about missing.
    expect(
      await axe(document.body, { rules: { region: { enabled: false } } }),
    ).toHaveNoViolations();
  });
});
