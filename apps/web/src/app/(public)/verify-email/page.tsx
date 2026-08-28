"use client";

import { useSearchParams } from "next/navigation";
import { Suspense } from "react";

import { AuthCard } from "@/features/auth/AuthCard";
import { AuthShell } from "@/features/auth/AuthShell";
import { TokenConsumer } from "@/features/auth/TokenConsumer";
import { Banner, TextLink } from "@/shared/components";
import { confirmEmailVerification } from "@/lib/auth/api";

/**
 * The verification landing, from screens/verify-email.html.
 *
 * The token is consumed on arrival — the person's click on the email link is
 * the confirmation, and asking them to click again here would be a second
 * button for the same intent. Success routes to sign-in rather than a
 * dashboard: verifying an address does not sign anyone in, because the email
 * could have been opened on any machine.
 */
function VerifyEmail() {
  const token = useSearchParams().get("token") ?? "";

  return (
    <TokenConsumer
      token={token}
      consume={confirmEmailVerification}
      checking="Checking the link you opened. This takes a couple of seconds."
      requestHref="/login"
      requestLabel="Back to sign in"
      done={
        <div>
          <Banner tone="success">Your email address is confirmed.</Banner>
          <section className="mt-5">
            <h2 className="text-sm font-semibold">What this unlocks</h2>
            <ul className="mt-2 list-disc space-y-1 pl-5 text-sm text-fg-2">
              <li>
                Voice practice sessions with per-answer coaching and a private
                score.
              </li>
              <li>
                Screening invitations from employers can now reach this address.
              </li>
              <li>Password resets and one-time sign-in codes.</li>
            </ul>
          </section>
          <p className="mt-6 text-sm">
            <TextLink href="/login">Continue to sign in</TextLink>
          </p>
        </div>
      }
    />
  );
}

export default function VerifyEmailPage() {
  return (
    <AuthShell>
      <AuthCard title="Confirming your email address">
        <Suspense fallback={null}>
          <VerifyEmail />
        </Suspense>
      </AuthCard>
    </AuthShell>
  );
}
