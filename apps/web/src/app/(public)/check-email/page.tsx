"use client";

import { AuthCard } from "@/features/auth/AuthCard";
import { AuthShell } from "@/features/auth/AuthShell";
import { CheckEmailPanel } from "@/features/auth/CheckEmailPanel";
import { useSentEmail } from "@/features/auth/sentEmail";
import type { SentEmail } from "@/features/auth/sentEmail";
import { requestTokenEmail } from "@/lib/auth/api";

/**
 * The screen after an email goes out, ported from screens/check-email.html.
 *
 * What was sent comes from session storage, written by whichever form asked.
 * Arriving with nothing stored — a bookmark, a fresh tab — still renders,
 * with generic copy and the resend disabled, because there is no address to
 * resend to and inventing an input here would duplicate the form the person
 * should go back to.
 */

/** Where "start again" goes, per flow. */
const changeAddress: Record<SentEmail["kind"], string> = {
  password_reset: "/forgot-password",
  magic_link: "/magic-link",
  otp: "/otp",
  verify_email: "/register",
};

export default function CheckEmailPage() {
  const sent = useSentEmail();
  const kind = sent?.kind ?? "password_reset";

  return (
    <AuthShell>
      <AuthCard title="Check your email">
        <CheckEmailPanel
          kind={kind}
          email={sent?.email ?? ""}
          request={requestTokenEmail}
          changeAddressHref={changeAddress[kind]}
        />
      </AuthCard>
    </AuthShell>
  );
}
