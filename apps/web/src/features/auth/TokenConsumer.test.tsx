import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

import { ApiError } from "@/lib/api/client";

import { TokenConsumer } from "./TokenConsumer";

/**
 * The consume-on-arrival machine behind verify-email and magic-link.
 *
 * The distinct-outcomes promise lives or dies here: each refusal code must
 * reach its own screen, and the token must be presented exactly once however
 * often React renders.
 */

function trouble(code: string): ApiError {
  return new ApiError({ status: 422, code, message: "refused" });
}

const props = {
  checking: "Checking the link…",
  requestHref: "/forgot-password",
  requestLabel: "Send a new link",
};

describe("TokenConsumer", () => {
  it("shows the checking state, then the done state", async () => {
    let release: () => void = () => {};
    const consume = vi
      .fn()
      .mockReturnValue(new Promise<void>((resolve) => (release = resolve)));

    render(
      <TokenConsumer
        {...props}
        token="vrf_x"
        consume={consume}
        done={<p>All confirmed</p>}
      />,
    );

    expect(screen.getByText("Checking the link…")).toBeInTheDocument();
    release();
    expect(await screen.findByText("All confirmed")).toBeInTheDocument();
  });

  it("presents the token exactly once across re-renders", async () => {
    // A second presentation would be answered "already used", and the person
    // would be told their own click beat them.
    const consume = vi.fn().mockResolvedValue(undefined);
    const { rerender } = render(
      <TokenConsumer
        {...props}
        token="vrf_x"
        consume={consume}
        done={<p>done</p>}
      />,
    );
    await screen.findByText("done");
    rerender(
      <TokenConsumer
        {...props}
        token="vrf_x"
        consume={consume}
        done={<p>done</p>}
      />,
    );

    expect(consume).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["TOKEN_EXPIRED", /this link has expired/i],
    ["TOKEN_USED", /already been used/i],
    ["TOKEN_SUPERSEDED", /newer email has replaced/i],
    ["TOKEN_INVALID", /not valid/i],
  ])("gives %s its own screen", async (code, headline) => {
    const consume = vi.fn().mockRejectedValue(trouble(code));
    render(
      <TokenConsumer
        {...props}
        token="vrf_x"
        consume={consume}
        done={<p>done</p>}
      />,
    );

    expect(
      await screen.findByRole("heading", { name: headline }),
    ).toBeInTheDocument();
  });

  it("leads every dead state with what did not happen", async () => {
    const consume = vi.fn().mockRejectedValue(trouble("TOKEN_EXPIRED"));
    render(
      <TokenConsumer
        {...props}
        token="vrf_x"
        consume={consume}
        done={<p>done</p>}
      />,
    );

    expect(
      await screen.findByText(/nothing has changed on your account/i),
    ).toBeInTheDocument();
  });

  it("treats an absent token as invalid without calling anything", () => {
    const consume = vi.fn();
    render(
      <TokenConsumer
        {...props}
        token=""
        consume={consume}
        done={<p>done</p>}
      />,
    );

    expect(consume).not.toHaveBeenCalled();
    expect(
      screen.getByRole("heading", { name: /not valid/i }),
    ).toBeInTheDocument();
  });

  it("offers the way forward from a superseded link without a resend button", async () => {
    // The newer email already exists; sending another would supersede that
    // one too, and the person chasing their own resends never wins.
    const consume = vi.fn().mockRejectedValue(trouble("TOKEN_SUPERSEDED"));
    render(
      <TokenConsumer
        {...props}
        token="vrf_x"
        consume={consume}
        done={<p>done</p>}
      />,
    );

    await screen.findByRole("heading", { name: /newer email/i });
    expect(
      screen.queryByRole("link", { name: /send a new link/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /back to sign in/i }),
    ).toBeInTheDocument();
  });

  it("has no accessibility violations in the trouble state", async () => {
    const consume = vi.fn().mockRejectedValue(trouble("TOKEN_EXPIRED"));
    const { container } = render(
      <TokenConsumer
        {...props}
        token="vrf_x"
        consume={consume}
        done={<p>done</p>}
      />,
    );
    await screen.findByRole("heading", { name: /expired/i });
    expect(await axe(container)).toHaveNoViolations();
  });
});
