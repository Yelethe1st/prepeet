"use client";

import { useSearchParams } from "next/navigation";
import { Suspense } from "react";

import { ErrorScreen } from "@/features/errors/ErrorScreen";
import { SessionProvider, useSession } from "@/lib/auth/session";
import { ButtonLink, TextLink } from "@/shared/components";

/**
 * 403, from error-403.html: the destination a refused request sends someone
 * to, at the route information-architecture.md names.
 *
 * The first sentence carries the screen's whole job: nothing is wrong with
 * the account, this one route is closed to this session. A 403 that looks
 * like a crash teaches people to retry it; one that looks like a sign-out
 * makes them log out and back in for nothing.
 *
 * WEB-05 requires naming the capability required and what is currently held.
 * The requirement arrives in the query string from whoever was refused; what
 * is held comes from the session itself, in capability terms rather than a
 * role name, because capabilities are what the decision was actually made on.
 */
function AccessDenied() {
  const search = useSearchParams();
  const required = search.get("capability") ?? "";
  const from = search.get("from") ?? "";
  const reference = search.get("reference") ?? "";
  const session = useSession();

  // The held capabilities nearest the refused one: same first segment, so a
  // person refused tenant.member_manage sees which tenant.* they do hold and
  // can name the gap when they ask their admin for access.
  const area = required.split(".")[0] ?? "";
  const heldNearby =
    session.status === "signed-in" && session.user && area
      ? session.user.capabilities.filter((held) => held.startsWith(`${area}.`))
      : [];

  const facts = [
    from ? { label: "Route requested", value: from, mono: true } : null,
    required
      ? { label: "Required capability", value: required, mono: true }
      : null,
    required
      ? {
          label: "Held in this area",
          value: heldNearby.length > 0 ? heldNearby.join(", ") : "none",
          mono: true,
        }
      : null,
    {
      label: "Decision",
      value: "Refused at the authorization boundary. No data was read.",
    },
    reference ? { label: "Reference", value: reference, mono: true } : null,
  ].filter((fact) => fact !== null);

  return (
    <ErrorScreen
      badge="403 · permission denied"
      title="You cannot open this page"
      actions={
        <>
          <ButtonLink href="/practice" variant="primary">
            Back to your dashboard
          </ButtonLink>
          <TextLink href="/login">Sign in as someone else</TextLink>
        </>
      }
      facts={facts}
      factsTitle="What was requested, and why it was refused"
    >
      <p>
        Nothing has gone wrong with your account — this one route is closed to
        your session. If you need it, your workspace admin can grant the
        capability named below.
      </p>
    </ErrorScreen>
  );
}

export default function AccessDeniedPage() {
  return (
    // Its own provider: this route lives in the public group, where no layout
    // supplies a session, but the person arriving here is usually signed in
    // and the screen is better for knowing what they hold.
    <SessionProvider>
      <Suspense fallback={null}>
        <AccessDenied />
      </Suspense>
    </SessionProvider>
  );
}
