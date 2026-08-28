import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

import { ApiError } from "@/lib/api/client";

import { RegisterForm } from "./RegisterForm";

/**
 * The registration form, ported from screens/register.html.
 *
 * The prototype switches between a candidate and an organisation account with
 * a radio group, and the organisation branch asks for a workspace name. Both
 * are here because the API supports both, and organisation registration is
 * what creates the tenant and its owning membership.
 */

const register = vi.fn();
const onRegistered = vi.fn();

beforeEach(() => {
  register.mockReset();
  onRegistered.mockReset();
});

function renderForm() {
  return render(
    <RegisterForm register={register} onRegistered={onRegistered} />,
  );
}

async function fill(
  email = "daniel.okonkwo@example.com",
  password = "a-long-enough-password",
) {
  await userEvent.type(screen.getByLabelText(/email/i), email);
  await userEvent.type(screen.getByLabelText(/^password$/i), password);
}

describe("RegisterForm", () => {
  it("registers a candidate by default, because that is the common case", async () => {
    register.mockResolvedValue(undefined);
    renderForm();

    await fill();
    await userEvent.click(screen.getByRole("button", { name: /create/i }));

    expect(register).toHaveBeenCalledWith({
      email: "daniel.okonkwo@example.com",
      password: "a-long-enough-password",
      account_type: "candidate",
    });
  });

  it("does not ask a candidate for an organisation name", () => {
    renderForm();

    expect(
      screen.queryByLabelText(/organisation name/i),
    ).not.toBeInTheDocument();
  });

  it("asks for the organisation name once that account type is chosen", async () => {
    renderForm();

    await userEvent.click(
      screen.getByRole("radio", { name: /screen candidates/i }),
    );

    expect(
      await screen.findByLabelText(/organisation name/i),
    ).toBeInTheDocument();
  });

  it("sends the organisation name with an organisation registration", async () => {
    register.mockResolvedValue(undefined);
    renderForm();

    await userEvent.click(
      screen.getByRole("radio", { name: /screen candidates/i }),
    );
    await fill();
    await userEvent.type(
      await screen.findByLabelText(/organisation name/i),
      "Northwind Recruiting",
    );
    await userEvent.click(screen.getByRole("button", { name: /create/i }));

    expect(register).toHaveBeenCalledWith({
      email: "daniel.okonkwo@example.com",
      password: "a-long-enough-password",
      account_type: "organisation",
      organisation_name: "Northwind Recruiting",
    });
  });

  /**
   * The server answers identically whether or not the address is already
   * registered, so that nobody can use this endpoint to discover who practises
   * for interviews. A form that said "check your inbox" only for new accounts
   * would give that away from the outside.
   */
  it("says the same thing whether or not the address was already registered", async () => {
    register.mockResolvedValue(undefined);
    renderForm();

    await fill();
    await userEvent.click(screen.getByRole("button", { name: /create/i }));

    const confirmation = await screen.findByRole("status");
    expect(confirmation).toHaveTextContent(/check/i);
    for (const leak of [
      /already/i,
      /existing account/i,
      /new account/i,
      /created/i,
    ]) {
      expect(confirmation).not.toHaveTextContent(leak);
    }
  });

  /**
   * Registration deliberately does not sign anybody in: verification comes
   * first. A form that navigated to the dashboard would be asserting a session
   * that does not exist.
   */
  it("does not hand control onward as though signed in", async () => {
    register.mockResolvedValue(undefined);
    renderForm();

    await fill();
    await userEvent.click(screen.getByRole("button", { name: /create/i }));

    await screen.findByRole("status");
    expect(onRegistered).toHaveBeenCalledWith("daniel.okonkwo@example.com");
  });

  it("puts a field error beside the field it belongs to", async () => {
    register.mockRejectedValue(
      new ApiError({
        status: 400,
        code: "VALIDATION_FAILED",
        message: "Some of the details were not accepted.",
        fieldErrors: { password: "A password needs at least 12 characters." },
      }),
    );
    renderForm();

    await fill();
    await userEvent.click(screen.getByRole("button", { name: /create/i }));

    const password = screen.getByLabelText(/^password$/i);
    await waitFor(() =>
      expect(password).toHaveAttribute("aria-invalid", "true"),
    );
    expect(password).toHaveAccessibleDescription(/at least 12 characters/i);
  });

  it("reports an organisation name error against that field", async () => {
    register.mockRejectedValue(
      new ApiError({
        status: 400,
        code: "VALIDATION_FAILED",
        message: "Some of the details were not accepted.",
        fieldErrors: {
          organisation_name: "An organisation registration needs a name.",
        },
      }),
    );
    renderForm();

    await userEvent.click(
      screen.getByRole("radio", { name: /screen candidates/i }),
    );
    await fill();
    await userEvent.type(
      await screen.findByLabelText(/organisation name/i),
      "x",
    );
    await userEvent.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() =>
      expect(screen.getByLabelText(/organisation name/i)).toHaveAttribute(
        "aria-invalid",
        "true",
      ),
    );
  });

  it("cannot be submitted twice while the first attempt is in flight", async () => {
    let release: () => void = () => {};
    register.mockReturnValue(
      new Promise<void>((resolve) => {
        release = resolve;
      }),
    );
    renderForm();

    await fill();
    await userEvent.click(screen.getByRole("button", { name: /create/i }));

    const button = screen.getByRole("button", { name: /creating/i });
    expect(button).toBeDisabled();
    await userEvent.click(button);
    expect(register).toHaveBeenCalledOnce();

    release();
  });

  it("asks the browser for a new password rather than an existing one", () => {
    renderForm();

    expect(screen.getByLabelText(/^password$/i)).toHaveAttribute(
      "autocomplete",
      "new-password",
    );
  });

  it("does not render the password as text", async () => {
    register.mockResolvedValue(undefined);
    const { container } = renderForm();

    await fill("a@b.co", "the-secret-password");

    expect(container.innerHTML).not.toContain("the-secret-password");
  });

  it("has no accessibility violations", async () => {
    const { container } = renderForm();

    expect(await axe(container)).toHaveNoViolations();
  });

  it("has no accessibility violations in the organisation branch", async () => {
    const { container } = renderForm();

    await userEvent.click(
      screen.getByRole("radio", { name: /screen candidates/i }),
    );
    await screen.findByLabelText(/organisation name/i);

    expect(await axe(container)).toHaveNoViolations();
  });
});

describe("RegisterForm with an unexpected failure", () => {
  it("shows a message of its own rather than the exception's", async () => {
    register.mockRejectedValue(
      new TypeError("Cannot read properties of undefined"),
    );
    renderForm();

    await fill();
    await userEvent.click(screen.getByRole("button", { name: /create/i }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/something went wrong/i);
    expect(alert).not.toHaveTextContent(/TypeError|undefined/);
  });

  it("does not report success when the call threw", async () => {
    register.mockRejectedValue(new TypeError("boom"));
    renderForm();

    await fill();
    await userEvent.click(screen.getByRole("button", { name: /create/i }));

    await screen.findByRole("alert");
    expect(onRegistered).not.toHaveBeenCalled();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("offers the correlation identifier when the server sent one", async () => {
    register.mockRejectedValue(
      new ApiError({
        status: 500,
        message: "Something went wrong.",
        requestId: "req_01a03",
      }),
    );
    renderForm();

    await fill();
    await userEvent.click(screen.getByRole("button", { name: /create/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent("req_01a03");
  });
});

describe("RegisterForm from the keyboard", () => {
  it("changes account type with the arrow keys, as a radio group should", async () => {
    renderForm();

    const candidate = screen.getByRole("radio", {
      name: /practise interviews/i,
    });
    candidate.focus();
    await userEvent.keyboard("{ArrowDown}");

    expect(
      screen.getByRole("radio", { name: /screen candidates/i }),
    ).toBeChecked();
    expect(
      await screen.findByLabelText(/organisation name/i),
    ).toBeInTheDocument();
  });

  it("submits on Enter", async () => {
    register.mockResolvedValue(undefined);
    renderForm();

    await fill();
    await userEvent.keyboard("{Enter}");

    await waitFor(() => expect(register).toHaveBeenCalledOnce());
  });
});
