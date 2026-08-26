import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

/** The member administration calls, typed from the contract. */

export type Member = components["schemas"]["Member"];
export type TenantRole = components["schemas"]["TenantRole"];

export async function listMembers(): Promise<Member[]> {
  const list =
    await apiFetch<components["schemas"]["MemberList"]>("/tenant/members");
  return list.members;
}

export async function inviteMember(
  email: string,
  role: string,
): Promise<Member> {
  return apiFetch<Member>("/tenant/members", {
    method: "POST",
    body: { email, role },
  });
}

export async function changeMemberRole(
  membershipId: string,
  role: string,
  expectedVersion: number,
): Promise<Member> {
  return apiFetch<Member>(`/tenant/members/${membershipId}`, {
    method: "PATCH",
    body: { role, expected_version: expectedVersion },
  });
}

export async function revokeMember(
  membershipId: string,
  expectedVersion: number,
): Promise<void> {
  await apiFetch(
    `/tenant/members/${membershipId}?expectedVersion=${expectedVersion}`,
    {
      method: "DELETE",
    },
  );
}
