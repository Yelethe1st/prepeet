"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";

import { ApiError } from "@/lib/api/client";

import type { CurrentUser } from "./api";
import { currentUser } from "./api";

/**
 * Who is signed in.
 *
 * A provider rather than a fetch in each component, because the answer is the
 * same for all of them and asking per component means a request per component
 * on every render.
 */

/**
 * The states a session can be in, and why there are four rather than two.
 *
 * `loading` exists because without it every consumer sees "signed out" for the
 * first frame, and a shell rendered from that flashes the signed out view before
 * the signed in one on every page load.
 *
 * `unavailable` exists because an outage is not being signed out. Treating it as
 * one would send somebody to a sign-in screen because the network blinked, and
 * the sign-in would fail too.
 */
export type SessionStatus = "loading" | "signed-in" | "signed-out" | "unavailable";

export interface Session {
  status: SessionStatus;
  user: SessionUser | null;
  /** Whether this session holds a capability. False while unknown. */
  can: (capability: string) => boolean;
  /** Re-reads the session, for after something that changes it. */
  refresh: () => Promise<void>;
}

/** The person, in the shape components want rather than the wire's. */
export interface SessionUser {
  id: string;
  email: string;
  emailVerified: boolean;
  activeTenantId: string | null;
  memberships: { tenantId: string; tenantName: string; status: string }[];
  capabilities: string[];
}

const SessionContext = createContext<Session | null>(null);

function toSessionUser(response: CurrentUser): SessionUser {
  return {
    id: response.user_id,
    email: response.email ?? "",
    emailVerified: response.email_verified,
    activeTenantId: response.active_tenant_id ?? null,
    memberships: (response.memberships ?? []).map((membership) => ({
      tenantId: membership.tenant_id,
      tenantName: membership.tenant_name,
      status: membership.status,
    })),
    capabilities: response.capabilities ?? [],
  };
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<SessionStatus>("loading");
  const [user, setUser] = useState<SessionUser | null>(null);

  /**
   * Written with callbacks rather than await, deliberately.
   *
   * React's set-state-in-effect rule refuses state updates reached from an
   * effect body, and its own guidance names the exception: updating state from a
   * callback, when an external system answers. That is exactly what this is, and
   * expressing it as a callback says so to the compiler as well as to a reader.
   */
  const load = useCallback(
    () =>
      currentUser().then(
        (response) => {
          setUser(toSessionUser(response));
          setStatus("signed-in");
        },
        (error: unknown) => {
          setUser(null);
          // Only a refusal means signed out. Anything else is the product being
          // unreachable, which needs a different screen and a different message.
          setStatus(
            error instanceof ApiError && error.status === 401 ? "signed-out" : "unavailable",
          );
        },
      ),
    [],
  );

  useEffect(() => {
    void load();
  }, [load]);

  const value = useMemo<Session>(
    () => ({
      status,
      user,
      // Deny by default reaches the browser too. While the answer is unknown
      // nothing is offered, so the shell cannot flash a control the person
      // turns out not to hold.
      can: (capability: string) => user?.capabilities.includes(capability) ?? false,
      refresh: load,
    }),
    [status, user, load],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

/**
 * useSession returns the current session.
 *
 * It throws outside a provider rather than returning a default. A default that
 * said "signed out" would render the signed out view forever, with nothing
 * explaining why, which is a worse failure than a stack trace naming the
 * missing provider.
 */
export function useSession(): Session {
  const session = useContext(SessionContext);
  if (session === null) {
    throw new Error("useSession was called outside a SessionProvider");
  }
  return session;
}
