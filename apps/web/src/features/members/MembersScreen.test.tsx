import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";
import { ApiError } from "@/lib/api/client";

import { MembersScreen } from "./MembersScreen";
import * as api from "./api";

vi.mock("./api");

const canHeld = new Set<string>();
vi.mock("@/lib/auth/session", () => ({
  useSession: () => ({
    status: "signed-in",
    user: null,
    can: (capability: string) => canHeld.has(capability),
    refresh: async () => {},
  }),
}));

/**
 * The members screen: TEN-02's surface. What is pinned here: a read-only
 * visitor sees the workspace without controls rather than a broken form,
 * the owner row offers no controls to anybody, every act carries the
 * version it read, and the matrix on the page is the generated one.
 */

const owner: api.Member = {
  membership_id: "00000000-0000-7000-8000-0000000000b0",
  user_id: "00000000-0000-7000-8000-0000000000c0",
  email: "amara@org.example",
  role: "owner",
  status: "active",
  version: 1,
  created_at: "2026-08-26T09:00:00Z",
};

const recruiter: api.Member = {
  membership_id: "00000000-0000-7000-8000-0000000000b1",
  user_id: "00000000-0000-7000-8000-0000000000c1",
  email: "priya@org.example",
  role: "recruiter",
  status: "active",
  version: 3,
  created_at: "2026-08-26T10:00:00Z",
};

function renderScreen(members: api.Member[] = [owner, recruiter]) {
  vi.mocked(api.listMembers).mockResolvedValue(members);
  return render(<MembersScreen />, {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryProvider>{children}</QueryProvider>
    ),
  });
}

afterEach(() => {
  vi.mocked(api.listMembers).mockReset();
  vi.mocked(api.inviteMember).mockReset();
  vi.mocked(api.changeMemberRole).mockReset();
  vi.mocked(api.revokeMember).mockReset();
  canHeld.clear();
});

describe("reading", () => {
  it("shows everyone with their role and status", async () => {
    canHeld.add("tenant.member_read");
    renderScreen();

    const row = within(await screen.findByRole("row", { name: /priya/i }));
    expect(row.getByText(/recruiter/i)).toBeInTheDocument();
    expect(row.getByText(/active/i)).toBeInTheDocument();
  });

  it("a read-only visitor sees the workspace without controls, not a broken form", async () => {
    canHeld.add("tenant.member_read");
    renderScreen();

    await screen.findByRole("row", { name: /priya/i });
    expect(
      screen.queryByRole("button", { name: /invite/i }),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /revoke/i }),
    ).not.toBeInTheDocument();
    expect(screen.getByText(/tenant admin role/i)).toBeInTheDocument();
  });

  it("renders the matrix from the catalogue, not from a table somebody typed", async () => {
    canHeld.add("tenant.member_read");
    renderScreen();
    await screen.findByRole("row", { name: /priya/i });

    const matrix = screen.getByRole("table", { name: /permission matrix/i });
    // Spot checks that trace to the generated bundles.
    const row = within(matrix).getByRole("row", { name: /rubric\.publish/i });
    expect(within(row).getAllByText(/^yes$/i).length).toBeGreaterThan(0);
    expect(within(row).getAllByText(/^no$/i).length).toBeGreaterThan(0);
  });
});

describe("administering", () => {
  it("invites by email and role", async () => {
    canHeld.add("tenant.member_read").add("tenant.member_manage");
    vi.mocked(api.inviteMember).mockResolvedValue({
      ...recruiter,
      membership_id: "00000000-0000-7000-8000-0000000000b2",
      email: "daniel@org.example",
      status: "invited",
    });
    const user = userEvent.setup();
    renderScreen();

    await user.type(
      await screen.findByLabelText(/email/i),
      "daniel@org.example",
    );
    await user.selectOptions(
      screen.getByLabelText(/^role$/i),
      "hiring_manager",
    );
    await user.click(screen.getByRole("button", { name: /invite/i }));

    expect(api.inviteMember).toHaveBeenCalledWith(
      "daniel@org.example",
      "hiring_manager",
    );
    await waitFor(() => expect(api.listMembers).toHaveBeenCalledTimes(2));
  });

  it("a refused invitation shows the field's own message and loses nothing", async () => {
    canHeld.add("tenant.member_read").add("tenant.member_manage");
    vi.mocked(api.inviteMember).mockRejectedValue(
      new ApiError({
        status: 400,
        message: "Some of the details were not accepted.",
        fieldErrors: { email: "No active account has that address." },
      }),
    );
    const user = userEvent.setup();
    renderScreen();

    await user.type(
      await screen.findByLabelText(/email/i),
      "ghost@org.example",
    );
    await user.click(screen.getByRole("button", { name: /invite/i }));

    expect(await screen.findByText(/no active account/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/email/i)).toHaveValue("ghost@org.example");
  });

  it("changes a role carrying the version it read", async () => {
    canHeld.add("tenant.member_read").add("tenant.member_manage");
    vi.mocked(api.changeMemberRole).mockResolvedValue({
      ...recruiter,
      role: "viewer",
      version: 4,
    });
    const user = userEvent.setup();
    renderScreen();

    const row = within(await screen.findByRole("row", { name: /priya/i }));
    await user.selectOptions(row.getByRole("combobox"), "viewer");

    expect(api.changeMemberRole).toHaveBeenCalledWith(
      recruiter.membership_id,
      "viewer",
      3,
    );
  });

  it("revokes with the version guard and explains what a stale read means", async () => {
    canHeld.add("tenant.member_read").add("tenant.member_manage");
    vi.mocked(api.revokeMember).mockRejectedValue(
      new ApiError({
        status: 409,
        message: "That membership changed since it was read.",
      }),
    );
    const user = userEvent.setup();
    renderScreen();

    const row = within(await screen.findByRole("row", { name: /priya/i }));
    await user.click(row.getByRole("button", { name: /revoke/i }));

    expect(api.revokeMember).toHaveBeenCalledWith(recruiter.membership_id, 3);
    expect(
      await screen.findByText(/changed since it was read/i),
    ).toBeInTheDocument();
  });

  it("the owner row offers no controls to anybody", async () => {
    canHeld.add("tenant.member_read").add("tenant.member_manage");
    renderScreen();

    const row = within(await screen.findByRole("row", { name: /amara/i }));
    expect(row.queryByRole("combobox")).not.toBeInTheDocument();
    expect(
      row.queryByRole("button", { name: /revoke/i }),
    ).not.toBeInTheDocument();
    expect(row.getByText(/owner/i)).toBeInTheDocument();
  });
});
