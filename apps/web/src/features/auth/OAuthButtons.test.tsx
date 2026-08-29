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

    await waitFor(() => expect(api.listOAuthProviders).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
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
