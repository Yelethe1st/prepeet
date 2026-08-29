import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const push = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push }),
  usePathname: () => "/practice",
}));

const currentUser = vi.fn();
const signOut = vi.fn();
const setActiveTenant = vi.fn();
vi.mock("@/lib/auth/api", () => ({
  currentUser: () => currentUser(),
  signOut: () => signOut(),
  setActiveTenant: (id: string | null) => setActiveTenant(id),
}));

import { ApiError } from "@/lib/api/client";

import AuthenticatedLayout from "./layout";

/**
 * Everything behind a session.
 *
 * Three outcomes, and each shows something different: signed in shows the
 * shell, signed out sends somebody to sign in, and unreachable says so. The
 * third exists because an outage is not being signed out, and sending somebody
 * to a sign-in screen they cannot use is worse than telling them the truth.
 */

beforeEach(() => {
  push.mockReset();
  currentUser.mockReset();
  signOut.mockReset();
  setActiveTenant.mockReset();
});

const user = {
  user_id: "usr_1",
  email: "daniel.okonkwo@example.com",
  email_verified: true,
  active_tenant_id: null,
  memberships: [],
  capabilities: ["candidate.practice.read_own"],
};

/**
 * A fresh client per test, with retries off.
 *
 * Fresh because a shared cache carries one test's answer into the next, and the
 * first to run would decide what the others saw. Retries off because a test
 * asserting the unreachable state should not wait out a backoff it did not ask
 * for.
 *
 * The provider is supplied here rather than by the layout, because the layout
 * is mounted under the root layout in the running application and a second
 * client would mean a second cache.
 */
function renderLayout() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });

  return {
    client,
    ...render(
      <QueryClientProvider client={client}>
        <AuthenticatedLayout>
          <h1>Practice</h1>
        </AuthenticatedLayout>
      </QueryClientProvider>,
    ),
  };
}

describe("AuthenticatedLayout", () => {
  it("renders the page inside the shell once the session is known", async () => {
    currentUser.mockResolvedValue(user);
    renderLayout();

    await waitFor(() => expect(screen.getByRole("main")).toBeInTheDocument());
    expect(
      screen.getByRole("heading", { level: 1, name: "Practice" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("navigation", { name: "Main" }),
    ).toBeInTheDocument();
  });

  /**
   * Nothing is rendered while the session is unknown. Rendering the shell first
   * would show navigation built from no capabilities, which is an empty sidebar
   * that fills in a moment later.
   */
  it("renders nothing while the session is still unknown", () => {
    currentUser.mockReturnValue(new Promise(() => {}));
    const { container } = renderLayout();

    expect(container).toBeEmptyDOMElement();
  });

  it("sends somebody without a session to sign in", async () => {
    currentUser.mockRejectedValue(
      new ApiError({ status: 401, message: "Please sign in again." }),
    );
    renderLayout();

    await waitFor(() => expect(push).toHaveBeenCalledWith("/login"));
  });

  it("does not render the page to somebody without a session", async () => {
    currentUser.mockRejectedValue(
      new ApiError({ status: 401, message: "Please sign in again." }),
    );
    const { container } = renderLayout();

    await waitFor(() => expect(push).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
  });

  /**
   * An outage is not being signed out. Sending somebody to a sign-in screen
   * because the network blinked would fail there too, and they would have no
   * idea why.
   */
  it("says the product is unreachable rather than sending somebody to sign in", async () => {
    currentUser.mockRejectedValue(
      new ApiError({ status: 0, message: "no network", offline: true }),
    );
    renderLayout();

    expect(
      await screen.findByRole("heading", { name: /not reachable/i }),
    ).toBeInTheDocument();
    expect(push).not.toHaveBeenCalled();
  });

  it("re-reads the session after switching workspace", async () => {
    currentUser.mockResolvedValue({
      ...user,
      active_tenant_id: "t-a",
      memberships: [
        { tenant_id: "t-a", tenant_name: "Northwind", status: "active" },
        { tenant_id: "t-b", tenant_name: "Orbital", status: "active" },
      ],
    });
    setActiveTenant.mockResolvedValue({});
    renderLayout();

    const switcher = await screen.findByRole("combobox", {
      name: /workspace/i,
    });
    currentUser.mockClear();

    // Radix renders its options only once opened, so the choice is two steps
    // rather than one. A native select would take selectOptions; this is the
    // cost of the focus management and typeahead that come with it.
    const { default: userEvent } = await import("@testing-library/user-event");
    await userEvent.click(switcher);
    await userEvent.click(
      await screen.findByRole("option", { name: "Orbital" }),
    );

    await waitFor(() => expect(setActiveTenant).toHaveBeenCalledWith("t-b"));
    // Re-read rather than assumed: switching changes what the session may do,
    // and the navigation is built from that.
    await waitFor(() => expect(currentUser).toHaveBeenCalled());
  });

  /**
   * IAM-03's last criterion: switching cannot expose a resource from the
   * previous workspace, including through a cached read model.
   *
   * This is that cache. Every query key in the application is scoped by what it
   * reads rather than by whose it is, so ["sessions"] means the same thing in
   * both workspaces, and the client answers from cache before it revalidates.
   * The first paint after a switch was the previous tenant's data.
   */
  it("keeps nothing the previous workspace put in the cache", async () => {
    currentUser.mockResolvedValue({
      ...user,
      active_tenant_id: "t-a",
      memberships: [
        { tenant_id: "t-a", tenant_name: "Northwind", status: "active" },
        { tenant_id: "t-b", tenant_name: "Orbital", status: "active" },
      ],
    });
    setActiveTenant.mockResolvedValue({});
    const { client } = renderLayout();

    const switcher = await screen.findByRole("combobox", {
      name: /workspace/i,
    });
    // Something the first workspace read, cached under a key that says nothing
    // about which workspace it belongs to.
    client.setQueryData(["sessions"], [{ id: "northwinds-session" }]);

    const { default: userEvent } = await import("@testing-library/user-event");
    await userEvent.click(switcher);
    await userEvent.click(
      await screen.findByRole("option", { name: "Orbital" }),
    );

    await waitFor(() => expect(setActiveTenant).toHaveBeenCalledWith("t-b"));
    await waitFor(() =>
      expect(client.getQueryData(["sessions"])).toBeUndefined(),
    );
  });

  /**
   * Cleared wholesale rather than keyed per query, because a per-key discipline
   * is one somebody forgets and the forgotten key is the leak. This asserts the
   * rule rather than one instance of it.
   */
  it("keeps nothing at all, not merely the keys somebody thought of", async () => {
    currentUser.mockResolvedValue({
      ...user,
      active_tenant_id: "t-a",
      memberships: [
        { tenant_id: "t-a", tenant_name: "Northwind", status: "active" },
        { tenant_id: "t-b", tenant_name: "Orbital", status: "active" },
      ],
    });
    setActiveTenant.mockResolvedValue({});
    const { client } = renderLayout();

    const switcher = await screen.findByRole("combobox", {
      name: /workspace/i,
    });
    for (const key of [
      ["profile"],
      ["documents"],
      ["catalogue"],
      ["a-key-nobody-has-added-yet"],
    ]) {
      client.setQueryData(key, { from: "northwind" });
    }

    const { default: userEvent } = await import("@testing-library/user-event");
    await userEvent.click(switcher);
    await userEvent.click(
      await screen.findByRole("option", { name: "Orbital" }),
    );

    await waitFor(() => expect(setActiveTenant).toHaveBeenCalledWith("t-b"));
    await waitFor(() => {
      for (const key of [
        ["profile"],
        ["documents"],
        ["catalogue"],
        ["a-key-nobody-has-added-yet"],
      ]) {
        expect(client.getQueryData(key)).toBeUndefined();
      }
    });
  });
});
