"use client";

import Link from "next/link";

import { ErrorScreen } from "@/features/errors/ErrorScreen";
import { ButtonLink } from "@/shared/components";

/**
 * Authenticated, but no workspace: the account is fine and there is nowhere
 * tenant-shaped to go.
 *
 * This happens for real reasons that are nobody's error: a membership was
 * revoked, an organisation offboarded someone, an invitation has not arrived
 * yet. Collapsing it into 403 would tell the person they were refused
 * something, when the truth is there is nothing to refuse - and the way
 * forward differs completely: a 403 wants an admin to grant a capability, this
 * wants an invitation or a different way of using the product.
 *
 * The practice path is the genuine way forward, not a consolation: practice
 * belongs to the person, not to any workspace, and stays theirs whatever
 * happens to memberships.
 */
export default function NoWorkspacePage() {
  return (
    <ErrorScreen
      badge="workspace · none active"
      title="You are signed in, with no workspace to open"
      actions={
        <>
          <ButtonLink href="/practice" variant="primary">
            Practise for yourself
          </ButtonLink>
          <Link className="text-sm" href="/register">
            Set up an organisation
          </Link>
        </>
      }
    >
      <p>
        Your account is fine. It just is not a member of any organisation&apos;s workspace right
        now — memberships end when an organisation removes someone, and begin with an invitation
        from one.
      </p>
      <p>
        If you are expecting a workspace, ask the person who runs it to invite this email address.
        Invitations arrive by email and take effect the moment you accept.
      </p>
      <p>
        Your practice history is untouched by any of this. Practice belongs to you, not to a
        workspace, and no employer can see it either way.
      </p>
    </ErrorScreen>
  );
}
