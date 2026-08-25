"use client";

import { useRouter } from "next/navigation";

import { AuthCard } from "@/features/auth/AuthCard";
import { AuthShell } from "@/features/auth/AuthShell";
import { RequestEmailForm } from "@/features/auth/RequestEmailForm";
import { rememberSentEmail } from "@/features/auth/sentEmail";
import { requestTokenEmail } from "@/lib/auth/api";

/**
 * The recovery request, ported from screens/forgot-password.html.
 *
 * The page owns routing and nothing else: on success it remembers what was
 * sent, for the check-email screen, and goes there. The address travels
 * through session storage rather than the URL, so it reaches neither browser
 * history nor a server log.
 */
export default function ForgotPasswordPage() {
  const router = useRouter();

  return (
    <AuthShell>
      <AuthCard title="Reset your password">
        <RequestEmailForm
          kind="password_reset"
          request={requestTokenEmail}
          onSent={(email) => {
            rememberSentEmail({ kind: "password_reset", email });
            router.push("/check-email");
          }}
        />
      </AuthCard>
    </AuthShell>
  );
}
