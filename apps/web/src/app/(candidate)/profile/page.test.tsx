import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";

import { QueryProvider } from "@/lib/api/QueryProvider";

import ProfilePage from "./page";
import * as api from "@/features/profile/api";

vi.mock("@/features/profile/api");

/**
 * The profile destination: the page owns the heading and the promise; the CV
 * section under it is CvSection's own suite's subject.
 */
describe("ProfilePage", () => {
  function renderPage(): void {
    vi.mocked(api.listDocuments).mockReturnValue(new Promise(() => {}));
    render(<ProfilePage />, {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    });
  }

  it("has exactly one first-level heading", () => {
    renderPage();

    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
  });

  it("says extraction is assistive, never authoritative", () => {
    renderPage();

    // The stance PRO-04 exists to keep visible: the parsing proposes and the
    // person decides. The line most likely to be dropped in a redesign.
    expect(screen.getByText(/you decide/i)).toBeInTheDocument();
  });
});
