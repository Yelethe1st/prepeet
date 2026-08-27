import { afterEach, describe, expect, it, vi } from "vitest";

import { apiFetch } from "@/lib/api/client";

import {
  changeMemberRole,
  inviteMember,
  listMembers,
  revokeMember,
} from "./api";

vi.mock("@/lib/api/client", () => ({ apiFetch: vi.fn() }));

/**
 * Membership administration's calls. Every mutation carries the version it
 * read, which is what makes a concurrent change a refusal rather than a
 * silent overwrite: an omitted version here would be a lost update.
 */

afterEach(() => {
  vi.mocked(apiFetch).mockReset();
});

describe("listMembers", () => {
  it("unwraps the envelope", async () => {
    const members = [{ id: "m1" }];
    vi.mocked(apiFetch).mockResolvedValue({ members } as never);

    await expect(listMembers()).resolves.toBe(members);
    expect(apiFetch).toHaveBeenCalledWith("/tenant/members");
  });
});

describe("inviteMember", () => {
  it("posts the address and the role it is invited to", async () => {
    vi.mocked(apiFetch).mockResolvedValue({} as never);

    await inviteMember("ama@example.com", "recruiter");

    expect(apiFetch).toHaveBeenCalledWith("/tenant/members", {
      method: "POST",
      body: { email: "ama@example.com", role: "recruiter" },
    });
  });
});

describe("changeMemberRole", () => {
  it("carries the expected version, so a stale change is refused not applied", async () => {
    vi.mocked(apiFetch).mockResolvedValue({} as never);

    await changeMemberRole("m1", "admin", 3);

    expect(apiFetch).toHaveBeenCalledWith("/tenant/members/m1", {
      method: "PATCH",
      body: { role: "admin", expected_version: 3 },
    });
  });
});

describe("revokeMember", () => {
  it("names the version it read in the request itself", async () => {
    vi.mocked(apiFetch).mockResolvedValue(undefined as never);

    await revokeMember("m1", 4);

    expect(apiFetch).toHaveBeenCalledWith(
      "/tenant/members/m1?expectedVersion=4",
      { method: "DELETE" },
    );
  });
});
