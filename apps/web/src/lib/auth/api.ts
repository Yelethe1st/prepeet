import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

/**
 * The request shapes, taken from the contract rather than from the components
 * that happen to send them.
 *
 * An earlier version imported them from the forms, which made this file depend
 * on a feature and inverted the direction: the network layer described itself
 * in terms of a screen. The contract is what the server accepts, so it is what
 * these are.
 */
export type SignInCredentials = components["schemas"]["LoginRequest"];
export type Registration = components["schemas"]["RegisterRequest"];

/** The account kinds the contract accepts. */
export type AccountType = Registration["account_type"];

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
  return apiFetch<Session>("/auth/login", {
    method: "POST",
    body: credentials,
  });
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
export async function setActiveTenant(
  tenantId: string | null,
): Promise<Session> {
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

/**
 * The token flows: IAM-02's four proofs of address control.
 *
 * The request half is one call whatever the flow, because the server treats
 * them identically at the boundary: 202 whether or not the address exists,
 * and a 429 with a countdown when a recent email is still fresh.
 */
export type TokenEmailKind = components["schemas"]["TokenEmailKind"];

/** requestTokenEmail asks for a verification, recovery, sign-in or code email. */
export async function requestTokenEmail(
  kind: TokenEmailKind,
  email: string,
): Promise<void> {
  await apiFetch("/auth/email/request", {
    method: "POST",
    body: { kind, email },
  });
}

/**
 * confirmEmailVerification consumes a verification link.
 *
 * Failures carry the token outcome codes; the caller renders each as its own
 * state, because the prototype gives each its own screen.
 */
export async function confirmEmailVerification(token: string): Promise<void> {
  await apiFetch("/auth/email/verify", { method: "POST", body: { token } });
}

/** confirmPasswordReset sets the new password and revokes every session. */
export async function confirmPasswordReset(
  token: string,
  password: string,
): Promise<void> {
  await apiFetch("/auth/password/reset", {
    method: "POST",
    body: { token, password },
  });
}

/**
 * consumeMagicLink exchanges a sign-in link for a session.
 *
 * The cookies arrive exactly as login's do; the body describes the session.
 */
export async function consumeMagicLink(token: string): Promise<Session> {
  return apiFetch<Session>("/auth/magic/consume", {
    method: "POST",
    body: { token },
  });
}

/** consumeOtp exchanges an emailed six-digit code for a session. */
export async function consumeOtp(
  email: string,
  code: string,
): Promise<Session> {
  return apiFetch<Session>("/auth/otp/consume", {
    method: "POST",
    body: { email, code },
  });
}

/** The sign-in providers this deployment offers, for IAM-08. */
export type OAuthProviders = components["schemas"]["OAuthProviders"];
export type OAuthStart = components["schemas"]["OAuthStart"];

/**
 * Which providers to draw buttons for.
 *
 * An empty list is the ordinary answer for a deployment with none configured,
 * and the sign-in screen shows email and password alone rather than nothing.
 */
export async function listOAuthProviders(): Promise<OAuthProviders> {
  return apiFetch<OAuthProviders>("/auth/oauth/providers");
}

/** Begin a provider sign-in and get where to send the browser. */
export async function startOAuth(
  provider: string,
  redirectTo?: string,
): Promise<OAuthStart> {
  return apiFetch<OAuthStart>(`/auth/oauth/${provider}/start`, {
    method: "POST",
    body: redirectTo === undefined ? {} : { redirect_to: redirectTo },
  });
}

/**
 * Finish a provider sign-in. The cookies arrive on this response, exactly as
 * they do from signing in with a password.
 */
export type OAuthSession = components["schemas"]["OAuthSession"];

/**
 * Finish a provider sign-in. The cookies arrive on this response, exactly as
 * they do from signing in with a password, and `redirect_to` is where the
 * sign-in was started from.
 */
export async function completeOAuth(
  provider: string,
  state: string,
  code: string,
): Promise<OAuthSession> {
  return apiFetch<OAuthSession>(`/auth/oauth/${provider}/callback`, {
    method: "POST",
    body: { state, code },
  });
}
