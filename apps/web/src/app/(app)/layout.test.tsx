import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }), usePathname: () => "/practice" }));

const currentUser = vi.fn();
const signOut = vi.fn();
const setActiveTenant = vi.fn();
vi.mock("@/features/auth/api", () => ({
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

function renderLayout() {
  return render(
    <AuthenticatedLayout>
      <h1>Practice</h1>
    </AuthenticatedLayout>,
  );
}

describe("AuthenticatedLayout", () => {
  it("renders the page inside the shell once the session is known", async () => {
    currentUser.mockResolvedValue(user);
    renderLayout();

    await waitFor(() => expect(screen.getByRole("main")).toBeInTheDocument());
    expect(screen.getByRole("heading", { level: 1, name: "Practice" })).toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "Main" })).toBeInTheDocument();
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
    currentUser.mockRejectedValue(new ApiError({ status: 401, message: "Please sign in again." }));
    renderLayout();

    await waitFor(() => expect(push).toHaveBeenCalledWith("/login"));
  });

  it("does not render the page to somebody without a session", async () => {
    currentUser.mockRejectedValue(new ApiError({ status: 401, message: "Please sign in again." }));
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
    currentUser.mockRejectedValue(new ApiError({ status: 0, message: "no network", offline: true }));
    renderLayout();

    expect(await screen.findByRole("heading", { name: /not reachable/i })).toBeInTheDocument();
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

    const switcher = await screen.findByRole("combobox", { name: /workspace/i });
    currentUser.mockClear();

    const { default: userEvent } = await import("@testing-library/user-event");
    await userEvent.selectOptions(switcher, "t-b");

    await waitFor(() => expect(setActiveTenant).toHaveBeenCalledWith("t-b"));
    // Re-read rather than assumed: switching changes what the session may do,
    // and the navigation is built from that.
    await waitFor(() => expect(currentUser).toHaveBeenCalled());
  });
});
