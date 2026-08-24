import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

import type { Registration } from "./RegisterForm";
import type { SignInCredentials } from "./SignInForm";

/**
 * The authentication calls, typed from the contract.
 *
 * These live beside the feature that makes them rather than in the client,
 * because the client should not grow a method per endpoint: a change to one
 * feature's calls would then touch a file every other feature imports.
 */

/** The session description the server returns. Tokens are never in the body. */
export type Session = components["schemas"]["Session"];
export type CurrentUser = components["schemas"]["CurrentUser"];

/**
 * signIn exchanges credentials for a session.
 *
 * The returned description is deliberately not the session itself: the tokens
 * are set as HttpOnly cookies and never appear in the body, so there is nothing
 * here to store and nothing for a script to steal.
 */
export async function signIn(credentials: SignInCredentials): Promise<Session> {
  return apiFetch<Session>("/auth/login", { method: "POST", body: credentials });
}

/**
 * register creates an account, or does nothing, and reports neither.
 *
 * The response is the same for a new address and one that already exists,
 * because confirming it would let anyone discover who practises for interviews.
 */
export async function register(registration: Registration): Promise<void> {
  await apiFetch("/auth/register", { method: "POST", body: registration });
}

/** currentUser describes who is signed in. */
export async function currentUser(): Promise<CurrentUser> {
  return apiFetch<CurrentUser>("/me");
}

/** Membership list, for the workspace switcher. */
export type MembershipList = components["schemas"]["MembershipList"];

/**
 * setActiveTenant chooses which workspace the session acts under.
 *
 * null clears the selection. The server verifies the membership and refuses
 * with 403 when there is none, which is deliberately not 404: the workspace may
 * well exist, and saying otherwise would be a way to test which identifiers are
 * real.
 */
export async function setActiveTenant(tenantId: string | null): Promise<Session> {
  return apiFetch<Session>("/me/active-tenant", {
    method: "PUT",
    body: { tenant_id: tenantId },
  });
}

/** listMemberships returns the workspaces this person belongs to. */
export async function listMemberships(): Promise<MembershipList> {
  return apiFetch<MembershipList>("/me/memberships");
}

/** signOut ends the session and its refresh family. */
export async function signOut(): Promise<void> {
  await apiFetch("/auth/logout", { method: "POST" });
}
