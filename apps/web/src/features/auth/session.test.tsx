import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api/client";

import { SessionProvider, useSession } from "./session";

/**
 * Who is signed in, for the components that need to know.
 *
 * A provider rather than a fetch in each component, because the answer is the
 * same for every one of them and asking repeatedly would mean a request per
 * component on every render.
 */

const currentUser = vi.fn();
vi.mock("./api", () => ({ currentUser: () => currentUser() }));

beforeEach(() => {
  currentUser.mockReset();
});

function Probe() {
  const session = useSession();
  return (
    <div>
      <span data-testid="status">{session.status}</span>
      <span data-testid="email">{session.user?.email ?? ""}</span>
      <span data-testid="capabilities">{(session.user?.capabilities ?? []).join(",")}</span>
      <span data-testid="can-read-campaigns">{String(session.can("campaign.read"))}</span>
    </div>
  );
}

function renderProbe() {
  return render(
    <SessionProvider>
      <Probe />
    </SessionProvider>,
  );
}

describe("SessionProvider", () => {
  /**
   * The loading state is not a detail. Without it every consumer sees "signed
   * out" for the first frame, and a shell rendered from that flashes the signed
   * out view before the signed in one on every page load.
   */
  it("starts as loading rather than as signed out", () => {
    currentUser.mockReturnValue(new Promise(() => {}));
    renderProbe();

    expect(screen.getByTestId("status")).toHaveTextContent("loading");
  });

  it("reports the signed in person", async () => {
    currentUser.mockResolvedValue({
      user_id: "usr_1",
      email: "daniel.okonkwo@example.com",
      email_verified: true,
      memberships: [],
      capabilities: ["candidate.practice.read_own"],
    });
    renderProbe();

    await waitFor(() => expect(screen.getByTestId("status")).toHaveTextContent("signed-in"));
    expect(screen.getByTestId("email")).toHaveTextContent("daniel.okonkwo@example.com");
  });

  it("reports signed out when the session is refused", async () => {
    currentUser.mockRejectedValue(new ApiError({ status: 401, message: "Please sign in again." }));
    renderProbe();

    await waitFor(() => expect(screen.getByTestId("status")).toHaveTextContent("signed-out"));
  });

  /**
   * An outage is not being signed out. Treating it as one would send somebody
   * to a sign-in screen because the network blinked, and the sign-in would fail
   * too.
   */
  it("does not report signed out when the request could not be made", async () => {
    currentUser.mockRejectedValue(
      new ApiError({ status: 0, message: "We could not reach Prepeet.", offline: true }),
    );
    renderProbe();

    await waitFor(() => expect(screen.getByTestId("status")).toHaveTextContent("unavailable"));
  });

  it("treats a server failure as unavailable rather than as signed out", async () => {
    currentUser.mockRejectedValue(new ApiError({ status: 500, message: "Something went wrong." }));
    renderProbe();

    await waitFor(() => expect(screen.getByTestId("status")).toHaveTextContent("unavailable"));
  });
});

describe("can", () => {
  it("is true for a capability the session holds", async () => {
    currentUser.mockResolvedValue({
      user_id: "usr_1",
      email_verified: true,
      memberships: [],
      capabilities: ["campaign.read"],
    });
    renderProbe();

    await waitFor(() => expect(screen.getByTestId("can-read-campaigns")).toHaveTextContent("true"));
  });

  it("is false for one it does not", async () => {
    currentUser.mockResolvedValue({
      user_id: "usr_1",
      email_verified: true,
      memberships: [],
      capabilities: ["candidate.practice.read_own"],
    });
    renderProbe();

    await waitFor(() => expect(screen.getByTestId("status")).toHaveTextContent("signed-in"));
    expect(screen.getByTestId("can-read-campaigns")).toHaveTextContent("false");
  });

  /**
   * Deny by default reaches the browser too. While the answer is unknown,
   * nothing is offered, so a shell cannot flash a control that the person turns
   * out not to hold.
   */
  it("is false while the session is still loading", () => {
    currentUser.mockReturnValue(new Promise(() => {}));
    renderProbe();

    expect(screen.getByTestId("can-read-campaigns")).toHaveTextContent("false");
  });
});

describe("useSession outside a provider", () => {
  /**
   * A component reading the session without a provider would otherwise get a
   * default that says signed out, and would render the signed out view forever
   * with nothing explaining why.
   */
  it("fails loudly rather than reporting signed out", () => {
    const complain = vi.spyOn(console, "error").mockImplementation(() => {});

    expect(() => render(<Probe />)).toThrow(/SessionProvider/);

    complain.mockRestore();
  });
});
