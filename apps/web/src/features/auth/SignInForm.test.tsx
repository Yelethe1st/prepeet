import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

import { ApiError } from "@/lib/api/client";

import { SignInForm } from "./SignInForm";

/**
 * The sign-in form, ported from screens/login.html.
 *
 * The form is a component rather than page code so that it can be tested
 * without a router. What is asserted is what a person experiences: that a
 * failure says something useful, that it does not say more than it should, and
 * that the form cannot be submitted twice.
 */

const signIn = vi.fn();
const onSignedIn = vi.fn();

beforeEach(() => {
  signIn.mockReset();
  onSignedIn.mockReset();
});

function renderForm() {
  return render(<SignInForm signIn={signIn} onSignedIn={onSignedIn} />);
}

async function fillAndSubmit(email = "daniel.okonkwo@example.com", password = "a-long-password") {
  await userEvent.type(screen.getByLabelText(/email/i), email);
  await userEvent.type(screen.getByLabelText(/^password$/i), password);
  await userEvent.click(screen.getByRole("button", { name: /sign in/i }));
}

describe("SignInForm", () => {
  it("sends what the person typed", async () => {
    signIn.mockResolvedValue(undefined);
    renderForm();

    await fillAndSubmit();

    expect(signIn).toHaveBeenCalledWith({
      email: "daniel.okonkwo@example.com",
      password: "a-long-password",
    });
  });

  it("hands control onward once signed in", async () => {
    signIn.mockResolvedValue(undefined);
    renderForm();

    await fillAndSubmit();

    await waitFor(() => expect(onSignedIn).toHaveBeenCalledOnce());
  });

  /**
   * The server answers a wrong password and an unknown address identically, and
   * takes the same time doing it. A form that added "no account with that
   * email" would undo that from the outside.
   */
  it("shows the server's refusal without elaborating on it", async () => {
    signIn.mockRejectedValue(
      new ApiError({ status: 401, code: "UNAUTHENTICATED", message: "Those details did not sign you in." }),
    );
    renderForm();

    await fillAndSubmit();

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Those details did not sign you in.");
    for (const leak of [/no account/i, /not registered/i, /wrong password/i, /unknown/i]) {
      expect(alert).not.toHaveTextContent(leak);
    }
  });

  it("puts a field error beside the field it belongs to", async () => {
    signIn.mockRejectedValue(
      new ApiError({
        status: 400,
        code: "VALIDATION_FAILED",
        message: "Some of the details were not accepted.",
        fieldErrors: { email: "Enter the email address you registered with." },
      }),
    );
    renderForm();

    await fillAndSubmit();

    const email = screen.getByLabelText(/email/i);
    await waitFor(() => expect(email).toHaveAttribute("aria-invalid", "true"));
    expect(email).toHaveAccessibleDescription(/Enter the email address you registered with/);
  });

  /**
   * An outage and a refusal need different words. Telling somebody their
   * details were wrong when the API was unreachable sends them to reset a
   * password that was never the problem.
   */
  it("says the connection failed when it did, rather than blaming the details", async () => {
    signIn.mockRejectedValue(
      new ApiError({ status: 0, message: "We could not reach Prepeet.", offline: true }),
    );
    renderForm();

    await fillAndSubmit();

    expect(await screen.findByRole("alert")).toHaveTextContent(/could not reach/i);
  });

  it("offers the correlation identifier when there is one to quote", async () => {
    signIn.mockRejectedValue(
      new ApiError({ status: 500, message: "Something went wrong.", requestId: "req_01a03" }),
    );
    renderForm();

    await fillAndSubmit();

    expect(await screen.findByRole("alert")).toHaveTextContent("req_01a03");
  });

  it("does not invent an identifier when the server sent none", async () => {
    signIn.mockRejectedValue(new ApiError({ status: 500, message: "Something went wrong." }));
    renderForm();

    await fillAndSubmit();

    expect(await screen.findByRole("alert")).not.toHaveTextContent(/req_/);
  });

  /**
   * A slow network makes a person click again. Without this the second click
   * is a second request, and for endpoints that cost money that is the whole
   * problem.
   */
  it("cannot be submitted twice while the first attempt is in flight", async () => {
    let release: () => void = () => {};
    signIn.mockReturnValue(new Promise<void>((resolve) => { release = resolve; }));
    renderForm();

    await fillAndSubmit();

    const button = screen.getByRole("button", { name: /signing in/i });
    expect(button).toBeDisabled();
    await userEvent.click(button);
    expect(signIn).toHaveBeenCalledOnce();

    release();
  });

  it("clears a previous failure when the person tries again", async () => {
    signIn.mockRejectedValueOnce(new ApiError({ status: 401, message: "Those details did not sign you in." }));
    renderForm();

    await fillAndSubmit();
    await screen.findByRole("alert");

    signIn.mockResolvedValueOnce(undefined);
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));

    await waitFor(() => expect(screen.queryByRole("alert")).not.toBeInTheDocument());
  });

  /**
   * The password is the one value that must never be reachable from the DOM as
   * text, because a page with any script on it can read an input's value but
   * should not find it lying in the markup.
   */
  it("does not render the password as text", async () => {
    signIn.mockResolvedValue(undefined);
    const { container } = renderForm();

    await fillAndSubmit("a@b.co", "the-secret-password");

    expect(container.innerHTML).not.toContain("the-secret-password");
  });

  it("asks the browser for the right autofill", () => {
    renderForm();

    expect(screen.getByLabelText(/email/i)).toHaveAttribute("autocomplete", "username");
    expect(screen.getByLabelText(/^password$/i)).toHaveAttribute("autocomplete", "current-password");
  });

  it("has no accessibility violations", async () => {
    const { container } = renderForm();

    expect(await axe(container)).toHaveNoViolations();
  });
});
