import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";

import MembersPage from "./page";
import * as api from "@/features/members/api";

vi.mock("@/features/members/api");
vi.mock("@/lib/auth/session", () => ({
  useSession: () => ({
    status: "signed-in",
    user: null,
    can: () => false,
    refresh: async () => {},
  }),
}));

/** The members destination: heading and the access-is-logged promise. */
describe("MembersPage", () => {
  it("has one first-level heading and says access is logged", () => {
    vi.mocked(api.listMembers).mockReturnValue(new Promise(() => {}));
    render(<MembersPage />, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });

    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    expect(screen.getByText(/logged, not just granted/i)).toBeInTheDocument();
  });
});
