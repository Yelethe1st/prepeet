"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createContext, useContext, useMemo } from "react";
import type { ReactNode } from "react";

import { ApiError } from "@/lib/api/client";

import type { CurrentUser } from "./api";
import { currentUser } from "./api";

/**
 * Who is signed in.
 *
 * TanStack Query owns the fetching, per the technology baseline. What that buys
 * beyond an effect is the part that would otherwise be written by hand and
 * written differently each time: one request no matter how many components ask,
 * a cache that can be invalidated when something changes the session, and a
 * loading state that is not a boolean somebody forgot to reset.
 *
 * The context is kept on top of it so consumers ask a question about the
 * session rather than about a query. `can` is the whole reason: a component
 * should ask what it may do, not read a capability array out of a query result.
 */

/** The key this query is cached under, so anything that changes it can say so. */
export const sessionQueryKey = ["session"] as const;

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
export type SessionStatus =
  "loading" | "signed-in" | "signed-out" | "unavailable";

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
  const client = useQueryClient();

  const query = useQuery({
    queryKey: sessionQueryKey,
    queryFn: currentUser,
    // Retry policy is the client's, not this query's. Setting it here as well
    // overrode the client, which meant a test asking for no retries got two
    // anyway and timed out waiting for a backoff it had switched off.
    //
    // Not refetched on focus. The session changes when something in the product
    // changes it, and those places invalidate the query; refetching every time
    // somebody switches tab is a request per tab switch for an answer that has
    // not moved.
    refetchOnWindowFocus: false,
  });

  const value = useMemo<Session>(() => {
    const user = query.data ? toSessionUser(query.data) : null;

    // Only a refusal means signed out. Anything else is the product being
    // unreachable, which needs a different screen and a different message.
    let status: SessionStatus = "loading";
    if (query.isPending) {
      status = "loading";
    } else if (user) {
      status = "signed-in";
    } else {
      status =
        query.error instanceof ApiError && query.error.status === 401
          ? "signed-out"
          : "unavailable";
    }

    return {
      status,
      user,
      // Deny by default reaches the browser too. While the answer is unknown
      // nothing is offered, so the shell cannot flash a control the person
      // turns out not to hold.
      can: (capability: string) =>
        user?.capabilities.includes(capability) ?? false,
      refresh: async () => {
        await client.invalidateQueries({ queryKey: sessionQueryKey });
      },
    };
  }, [query.data, query.error, query.isPending, client]);

  return (
    <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
  );
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
