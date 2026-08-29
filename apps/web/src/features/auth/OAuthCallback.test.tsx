import { render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

import { ApiError } from "@/lib/api/client";
import * as api from "@/lib/auth/api";
import { QueryProvider } from "@/lib/api/QueryProvider";

import { OAuthCallback } from "./OAuthCallback";

vi.mock("@/lib/auth/api");

/**
 * IAM-08's callback screen. What it must get right is the failure: this is the
 * one screen somebody reaches having already left the product, so a dead end
 * here is a person who cannot sign in and has nothing to read.
 */
function renderCallback(
  props: Partial<Parameters<typeof OAuthCallback>[0]> = {},
) {
  const onSignedIn = vi.fn();
  render(
    <OAuthCallback
      provider="google"
      providerLabel="Google"
      state="the-state"
      code="the-code"
      providerError=""
      onSignedIn={onSignedIn}
      {...props}
    />,
    {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    },
  );
  return onSignedIn;
}

afterEach(() => {
  vi.mocked(api.completeOAuth).mockReset();
});

describe("the OAuth callback", () => {
  it("completes the sign-in on arrival, without asking anything", async () => {
    vi.mocked(api.completeOAuth).mockResolvedValue({} as never);

    const onSignedIn = renderCallback();

    await waitFor(() =>
      expect(api.completeOAuth).toHaveBeenCalledWith(
        "google",
        "the-state",
        "the-code",
      ),
    );
    await waitFor(() => expect(onSignedIn).toHaveBeenCalledWith("/practice"));
    // Nothing to press: the provider has already redirected.
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("names the provider while it waits", () => {
    vi.mocked(api.completeOAuth).mockReturnValue(new Promise(() => {}));

    renderCallback();

    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      /Completing sign-in with Google/,
    );
  });

  /**
   * The fifth criterion. A refusal must name what happened and offer email and
   * password, rather than leaving somebody on a spinner they cannot escape.
   */
  it("offers email and password when the sign-in is refused", async () => {
    vi.mocked(api.completeOAuth).mockRejectedValue(
      new ApiError({
        status: 409,
        code: "OAUTH_STATE_EXPIRED",
        message: "That sign-in took too long. Start again and it will work.",
        requestId: "req_8Kd2Ln4Q",
      }),
    );

    renderCallback();

    expect(await screen.findByText(/took too long/)).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /back to sign in/i }),
    ).toHaveAttribute("href", "/login");
    // The reference, so somebody contacting us can quote it.
    expect(screen.getByText("req_8Kd2Ln4Q")).toBeInTheDocument();
  });

  /** A provider that declines redirects with an error and no code. */
  it("treats a provider's own refusal as a failure rather than a missing code", async () => {
    renderCallback({ providerError: "access_denied", code: "" });

    expect(
      await screen.findByText(/Google did not complete the sign-in/),
    ).toBeInTheDocument();
    expect(api.completeOAuth).not.toHaveBeenCalled();
  });

  /** A truncated or hand-typed callback link. */
  it("does not call the server when the provider sent nothing back", async () => {
    renderCallback({ state: "", code: "" });

    expect(
      await screen.findByRole("heading", {
        name: /could not finish that sign-in/i,
      }),
    ).toBeInTheDocument();
    expect(api.completeOAuth).not.toHaveBeenCalled();
  });

  /**
   * All three reasons are shown because none of them can be told apart from
   * outside: the server answers one refusal for a forged state and a replayed
   * one on purpose, so the screen must not claim to know which.
   */
  it("says why this usually happens without claiming to know which", async () => {
    renderCallback({ state: "", code: "" });

    expect(
      await screen.findByText(/started in one tab and finished in another/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/consent screen and the authorisation code expired/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/copied and opened somewhere else/),
    ).toBeInTheDocument();
  });

  it("has no accessibility violations while working", async () => {
    vi.mocked(api.completeOAuth).mockReturnValue(new Promise(() => {}));

    const { container } = render(
      <OAuthCallback
        provider="google"
        providerLabel="Google"
        state="s"
        code="c"
        providerError=""
        onSignedIn={vi.fn()}
      />,
      {
        wrapper: ({ children }: { children: ReactNode }) => (
          <QueryProvider>{children}</QueryProvider>
        ),
      },
    );

    expect(await axe(container)).toHaveNoViolations();
  });
});
