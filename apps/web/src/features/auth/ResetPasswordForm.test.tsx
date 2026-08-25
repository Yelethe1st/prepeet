import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

import { ApiError } from "@/lib/api/client";

import { ResetPasswordForm } from "./ResetPasswordForm";

const strongPassword = "a completely new passphrase";

function renderForm(overrides: Partial<Parameters<typeof ResetPasswordForm>[0]> = {}) {
  const props = {
    token: "rst_x",
    reset: vi.fn().mockResolvedValue(undefined),
    onReset: vi.fn(),
    onTokenTrouble: vi.fn(),
    ...overrides,
  };
  render(<ResetPasswordForm {...props} />);
  return props;
}

async function fillAndSubmit(password = strongPassword, confirm = password) {
  await userEvent.type(screen.getByLabelText(/^new password$/i), password);
  await userEvent.type(screen.getByLabelText(/confirm/i), confirm);
  await userEvent.click(screen.getByRole("button", { name: /save new password/i }));
}

describe("ResetPasswordForm", () => {
  it("saves the new password with the link's token", async () => {
    const props = renderForm();
    await fillAndSubmit();

    expect(props.reset).toHaveBeenCalledWith("rst_x", strongPassword);
    expect(props.onReset).toHaveBeenCalled();
  });

  it("warns about other devices before the button, not after", () => {
    renderForm();
    // The reset revokes every session; somebody mid-interview elsewhere
    // deserves to know before they press save.
    expect(screen.getByText(/signs out every other device/i)).toBeInTheDocument();
  });

  it("shows the requirements meeting themselves as the person types", async () => {
    renderForm();

    expect(screen.getByText(/at least 12 characters — not met/i)).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText(/^new password$/i), strongPassword);
    expect(screen.getByText(/at least 12 characters — met/i)).toBeInTheDocument();

    expect(screen.getByText(/both entries match — not met/i)).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText(/confirm/i), strongPassword);
    expect(screen.getByText(/both entries match — met/i)).toBeInTheDocument();
  });

  it("claims only the rules the server enforces", () => {
    renderForm();
    // The prototype also promised mixed case, digits and breach checks, which
    // nothing enforces. A promise without its check teaches people the wrong
    // thing about what made their password acceptable.
    const requirements = screen.getByRole("list", { name: /password requirements/i });
    expect(requirements.children).toHaveLength(2);
  });

  it("keeps a mismatch on the confirmation field", async () => {
    const props = renderForm();
    await fillAndSubmit(strongPassword, "a different passphrase entirely");

    expect(props.reset).not.toHaveBeenCalled();
    expect(screen.getByText(/not the same/i)).toBeInTheDocument();
  });

  it("hands a dead token to the trouble screen rather than the form's banner", async () => {
    const props = renderForm({
      reset: vi.fn().mockRejectedValue(new ApiError({ status: 422, code: "TOKEN_EXPIRED", message: "x" })),
    });
    await fillAndSubmit();

    expect(props.onTokenTrouble).toHaveBeenCalledWith("TOKEN_EXPIRED");
  });

  it("keeps a non-token failure on the form", async () => {
    renderForm({
      reset: vi.fn().mockRejectedValue(new ApiError({ status: 500, code: "INTERNAL", message: "Something broke." })),
    });
    await fillAndSubmit();

    expect(screen.getByText(/something broke/i)).toBeInTheDocument();
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
