"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";

import { AuthCard } from "@/features/auth/AuthCard";
import { AuthShell } from "@/features/auth/AuthShell";
import { ResetPasswordForm } from "@/features/auth/ResetPasswordForm";
import { TokenTrouble } from "@/features/auth/TokenTrouble";
import type { TokenTroubleCode } from "@/features/auth/TokenTrouble";
import { Banner } from "@/shared/components";
import { confirmPasswordReset } from "@/lib/auth/api";

/**
 * The new-password screen, from screens/reset-password.html.
 *
 * Three states, all reachable only from here: the form, the success note with
 * its route back to sign-in, and the token-trouble screen when the link
 * itself was refused. Success does not sign the person in: the reset revoked
 * every session, and issuing a fresh one silently would make "signs out every
 * other device" untrue of the device most likely to be the attacker's own.
 */
function ResetPassword() {
  const token = useSearchParams().get("token") ?? "";
  const [phase, setPhase] = useState<
    | { state: "form" }
    | { state: "done" }
    | { state: "trouble"; code: TokenTroubleCode }
  >(token ? { state: "form" } : { state: "trouble", code: "TOKEN_INVALID" });

  if (phase.state === "trouble") {
    return (
      <TokenTrouble
        code={phase.code}
        requestHref="/forgot-password"
        requestLabel="Send a new link"
      />
    );
  }

  if (phase.state === "done") {
    return (
      <div aria-live="polite">
        <Banner tone="success">
          Your password is changed, and every other device is signed out.
        </Banner>
        <p className="mt-4 text-sm">
          <Link className="font-semibold" href="/login">
            Sign in with your new password
          </Link>
        </p>
      </div>
    );
  }

  return (
    <ResetPasswordForm
      token={token}
      reset={confirmPasswordReset}
      onReset={() => setPhase({ state: "done" })}
      onTokenTrouble={(code) => setPhase({ state: "trouble", code })}
    />
  );
}

export default function ResetPasswordPage() {
  return (
    <AuthShell>
      <AuthCard title="Choose a new password">
        {/* useSearchParams needs a boundary in the app router. */}
        <Suspense fallback={null}>
          <ResetPassword />
        </Suspense>
      </AuthCard>
    </AuthShell>
  );
}
