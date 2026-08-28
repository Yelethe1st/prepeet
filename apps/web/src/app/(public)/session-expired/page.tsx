"use client";

import Link from "next/link";

import { ErrorScreen } from "@/features/errors/ErrorScreen";
import { ButtonLink } from "@/shared/components";

/**
 * Expired authentication: the destination for a session that ended mid-use.
 *
 * Its own screen rather than a bounce to /login, because the two arrivals
 * mean different things: someone landing on /login chose to sign in, and
 * someone landing here was interrupted. The interruption deserves the
 * explanation - nothing they did, nothing is lost server-side - before the
 * form asks them to prove who they are again.
 *
 * Deliberately not one of TokenTrouble's states: those are about a link in an
 * email, this is about a session in a browser, and "request a new email"
 * would be exactly the wrong way forward here.
 */
export default function SessionExpiredPage() {
  return (
    <ErrorScreen
      badge="session · expired"
      title="Your session has ended"
      actions={
        <>
          <ButtonLink href="/login" variant="primary">
            Sign in again
          </ButtonLink>
          <Link className="text-sm" href="/magic-link">
            Email me a sign-in link instead
          </Link>
        </>
      }
    >
      <p>
        Sessions end after a fixed time, or when your password changes, or when
        you sign out on another device. It is not something you did.
      </p>
      <p>
        Nothing on the server was lost: anything you completed was saved when
        you completed it. Signing in again picks up where the record left off.
      </p>
    </ErrorScreen>
  );
}
