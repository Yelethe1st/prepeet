import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

/**
 * WEB-05's first criterion, tested as a matrix: the five destinations are
 * distinct and never collapse into one generic page. Each is rendered and
 * asserted on what makes it itself - the headline, the claim it leads with,
 * and the way forward it offers - and then all five headlines are asserted
 * different from each other, which is the collapse this exists to prevent.
 */

let search = new URLSearchParams();
const push = vi.fn();
vi.mock("next/navigation", () => ({
  useSearchParams: () => search,
  usePathname: () => "/candidate/sessions/archive",
  useRouter: () => ({ push }),
}));

// The 403 page mounts its own SessionProvider; a stubbed session keeps these
// tests about the destination rather than about fetching.
vi.mock("@/lib/auth/session", () => ({
  SessionProvider: ({ children }: { children: React.ReactNode }) => children,
  useSession: () => ({
    status: "signed-in",
    user: {
      capabilities: ["tenant.member_view", "candidate.practice.read_own"],
    },
  }),
}));

import ErrorBoundary from "@/app/error";
import NotFound from "@/app/not-found";
import AccessDeniedPage from "@/app/(public)/access-denied/page";
import NoWorkspacePage from "@/app/(public)/no-workspace/page";
import SessionExpiredPage from "@/app/(public)/session-expired/page";

beforeEach(() => {
  search = new URLSearchParams();
});

describe("the five destinations", () => {
  it("404 says nothing exists there and shows what was requested", () => {
    render(<NotFound />);

    expect(
      screen.getByRole("heading", { name: /could not find/i }),
    ).toBeInTheDocument();
    expect(screen.getByText("/candidate/sessions/archive")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /dashboard/i }),
    ).toBeInTheDocument();
  });

  it("500 leads with whose fault it was and carries the reference support acts on", () => {
    const error = Object.assign(new Error("boom"), { digest: "digest_7f3a" });
    render(<ErrorBoundary error={error} reset={vi.fn()} />);

    expect(screen.getByText(/not something you did/i)).toBeInTheDocument();
    // The criterion: a correlation identifier the support team can act on.
    expect(screen.getByText("digest_7f3a")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /retry/i })).toBeInTheDocument();
  });

  it("500 retries through the boundary's reset", async () => {
    const reset = vi.fn();
    const error = Object.assign(new Error("boom"), { digest: "d" });
    render(<ErrorBoundary error={error} reset={reset} />);

    screen.getByRole("button", { name: /retry/i }).click();
    expect(reset).toHaveBeenCalled();
  });

  it("403 names the required capability and what is held near it", () => {
    search = new URLSearchParams(
      "capability=tenant.member_manage&from=/admin/members&reference=req_01K3",
    );
    render(<AccessDeniedPage />);

    expect(
      screen.getByRole("heading", { name: /cannot open/i }),
    ).toBeInTheDocument();
    // The criterion, both halves: required, and currently held.
    expect(screen.getByText("tenant.member_manage")).toBeInTheDocument();
    expect(screen.getByText("tenant.member_view")).toBeInTheDocument();
    // Held-nearby filters to the refused area: the practice capability is not
    // an answer to a tenant question.
    expect(screen.queryByText(/practice.read_own/)).not.toBeInTheDocument();
    expect(screen.getByText("req_01K3")).toBeInTheDocument();
    // The reassurance that stops the log-out-and-back-in ritual.
    expect(
      screen.getByText(/nothing has gone wrong with your account/i),
    ).toBeInTheDocument();
  });

  it("403 says none rather than hiding the row when nothing nearby is held", () => {
    search = new URLSearchParams("capability=platform.audit_read");
    render(<AccessDeniedPage />);

    const row = screen.getByText("Held in this area").closest("div");
    expect(within(row as HTMLElement).getByText("none")).toBeInTheDocument();
  });

  it("session-expired is an interruption explained, not a login bounce", () => {
    render(<SessionExpiredPage />);

    expect(
      screen.getByRole("heading", { name: /session has ended/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/nothing on the server was lost/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /sign in again/i }),
    ).toHaveAttribute("href", "/login");
  });

  it("no-workspace is not a refusal, and practice is the real way forward", () => {
    render(<NoWorkspacePage />);

    expect(
      screen.getByRole("heading", { name: /no workspace/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/your account is fine/i)).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /practise for yourself/i }),
    ).toHaveAttribute("href", "/practice");
    // The separation, stated where somebody just lost a workspace and may
    // wonder what else went with it.
    expect(screen.getByText(/no employer can see it/i)).toBeInTheDocument();
  });

  it("never collapses two destinations into one page", () => {
    // The whole ticket in one assertion: five renders, five different
    // headlines. A refactor that routes two cases to one screen fails here
    // whatever the screens say.
    const headlines = new Set<string>();

    for (const destination of [
      () => render(<NotFound />),
      () =>
        render(
          <ErrorBoundary
            error={Object.assign(new Error("x"), { digest: "d" })}
            reset={vi.fn()}
          />,
        ),
      () => render(<AccessDeniedPage />),
      () => render(<SessionExpiredPage />),
      () => render(<NoWorkspacePage />),
    ]) {
      const { unmount } = destination();
      headlines.add(
        screen.getByRole("heading", { level: 1 }).textContent ?? "",
      );
      unmount();
    }

    expect(headlines.size).toBe(5);
  });
});
