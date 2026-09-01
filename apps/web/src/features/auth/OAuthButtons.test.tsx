import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "@/lib/auth/api";
import { QueryProvider } from "@/lib/api/QueryProvider";

import { OAuthButtons } from "./OAuthButtons";

vi.mock("@/lib/auth/api");

function renderButtons() {
  return render(<OAuthButtons />, {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryProvider>{children}</QueryProvider>
    ),
  });
}

beforeEach(() => {
  vi.stubGlobal("location", { assign: vi.fn() });
});

afterEach(() => {
  vi.mocked(api.listOAuthProviders).mockReset();
  vi.mocked(api.startOAuth).mockReset();
  vi.unstubAllGlobals();
});

describe("the sign-in provider buttons", () => {
  it("draws one button per configured provider", async () => {
    vi.mocked(api.listOAuthProviders).mockResolvedValue({
      providers: [
        { id: "google", label: "Google" },
        { id: "microsoft", label: "Microsoft" },
      ],
    });

    renderButtons();

    expect(
      await screen.findByRole("button", { name: /Continue with Google/ }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Continue with Microsoft/ }),
    ).toBeInTheDocument();
  });

  /**
   * IAM-08's sixth criterion, from the other side: a deployment with none
   * configured shows email and password alone, not an empty heading or a
   * divider with nothing above it.
   */
  it("renders nothing at all when no provider is configured", async () => {
    vi.mocked(api.listOAuthProviders).mockResolvedValue({ providers: [] });

    const { container } = renderButtons();

    // Waits for the empty container rather than for the call. The component
    // draws placeholders while it asks, so "has been called" is no longer the
    // moment the answer is known; the guarantee is what is left afterwards.
    await waitFor(() => expect(container).toBeEmptyDOMElement());
    expect(
      screen.queryByText(/or sign in with email/i),
    ).not.toBeInTheDocument();
  });

  it("sends the browser where the server said", async () => {
    const user = userEvent.setup();
    vi.mocked(api.listOAuthProviders).mockResolvedValue({
      providers: [{ id: "google", label: "Google" }],
    });
    vi.mocked(api.startOAuth).mockResolvedValue({
      authorization_url: "https://accounts.google.example/authorize?state=abc",
      state: "abc",
    });

    renderButtons();
    await user.click(
      await screen.findByRole("button", { name: /Continue with Google/ }),
    );

    await waitFor(() =>
      expect(window.location.assign).toHaveBeenCalledWith(
        "https://accounts.google.example/authorize?state=abc",
      ),
    );
  });

  /**
   * A provider that cannot be reached must not take the screen over: the form
   * beneath still works, so this says what failed and points at it.
   */
  it("keeps the form usable when a provider cannot be reached", async () => {
    const user = userEvent.setup();
    vi.mocked(api.listOAuthProviders).mockResolvedValue({
      providers: [{ id: "google", label: "Google" }],
    });
    vi.mocked(api.startOAuth).mockRejectedValue(new Error("offline"));

    renderButtons();
    await user.click(
      await screen.findByRole("button", { name: /Continue with Google/ }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /email and password/i,
    );
    expect(
      screen.getByRole("button", { name: /Continue with Google/ }),
    ).toBeEnabled();
  });
});

describe("the provider buttons as the prototype draws them", () => {
  it("gives each provider its brand mark", async () => {
    // login.html renders a letter badge in the provider's own colours beside
    // the label. It is decorative, so it is hidden from assistive technology:
    // the button already says "Continue with Google", and a screen reader
    // announcing "G Continue with Google" is worse, not more informative.
    vi.mocked(api.listOAuthProviders).mockResolvedValue({
      providers: [
        { id: "google", label: "Google" },
        { id: "microsoft", label: "Microsoft" },
      ],
    });

    renderButtons();

    const google = await screen.findByRole("button", {
      name: "Continue with Google",
    });
    const mark = google.querySelector("[data-provider-mark]");
    expect(mark).not.toBeNull();
    expect(mark).toHaveAttribute("aria-hidden", "true");
    expect(mark).toHaveTextContent("G");
  });

  it("marks an unknown provider with its own initial", async () => {
    // A deployment may configure a provider this build has never heard of.
    // Falling back to the first letter of its label is better than no mark and
    // far better than a hardcoded list that silently omits it.
    vi.mocked(api.listOAuthProviders).mockResolvedValue({
      providers: [{ id: "okta", label: "Okta (Northwind Health)" }],
    });

    renderButtons();

    const button = await screen.findByRole("button", {
      name: /Continue with Okta/,
    });
    expect(button.querySelector("[data-provider-mark]")).toHaveTextContent("O");
  });

  it("shows placeholders while the list is loading", async () => {
    // The prototype draws skeletons rather than nothing. A row of buttons that
    // appears a beat after somebody has already started typing their password
    // moves the form under their hands, and a person who never saw the options
    // cannot know they were offered.
    vi.mocked(api.listOAuthProviders).mockImplementation(
      () => new Promise(() => {}),
    );

    renderButtons();

    expect(
      await screen.findByText(/checking which sign-in options are available/i),
    ).toBeInTheDocument();
  });

  it("says nothing at all once it knows there are none", async () => {
    // The placeholders are a promise that something is coming. Leaving them up
    // when the answer is "none" would be a lie told in skeleton form.
    vi.mocked(api.listOAuthProviders).mockResolvedValue({ providers: [] });

    const { container } = renderButtons();

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });
});

describe("when the provider list will not load", () => {
  it("stops showing placeholders once the first attempt has failed", async () => {
    // react-query retries, so "still pending" is true long after the first
    // failure. A person whose API is unreachable would otherwise watch
    // skeleton buttons for as long as they cared to look, and the form
    // beneath works perfectly.
    vi.mocked(api.listOAuthProviders).mockRejectedValue(new Error("offline"));

    const { container } = renderButtons();

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });
});
