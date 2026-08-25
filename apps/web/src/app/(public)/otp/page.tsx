"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { AuthCard } from "@/features/auth/AuthCard";
import { AuthShell } from "@/features/auth/AuthShell";
import { OtpForm } from "@/features/auth/OtpForm";
import { RequestEmailForm } from "@/features/auth/RequestEmailForm";
import { rememberSentEmail, useSentEmail } from "@/features/auth/sentEmail";
import { TokenTrouble } from "@/features/auth/TokenTrouble";
import type { TokenTroubleCode } from "@/features/auth/TokenTrouble";
import { consumeOtp, requestTokenEmail } from "@/lib/auth/api";

/**
 * The one-time-code route, from screens/otp.html.
 *
 * Both halves of the flow live here: asking for the code and typing it. The
 * split is decided by whether this tab remembers sending one, so the person
 * who arrives fresh sees the request form and the person who just asked sees
 * the entry, with "request a fresh code" always one state away.
 */
export default function OtpPage() {
  const router = useRouter();
  const sent = useSentEmail();
  const [phase, setPhase] = useState<
    | { state: "enter"; email: string }
    | { state: "trouble"; code: TokenTroubleCode }
    | null
  >(null);

  // Until something happened on this screen, the stored send decides which
  // half shows: the person who just asked for a code sees the entry, anyone
  // else sees the request form.
  const resolved =
    phase ?? (sent?.kind === "otp" ? { state: "enter" as const, email: sent.email } : { state: "request" as const });

  return (
    <AuthShell>
      <AuthCard title="Enter your one-time code">
        {resolved.state === "request" ? (
          <RequestEmailForm
            kind="otp"
            request={requestTokenEmail}
            onSent={(email) => {
              rememberSentEmail({ kind: "otp", email });
              setPhase({ state: "enter", email });
            }}
          />
        ) : null}

        {resolved.state === "enter" ? (
          <OtpForm
            email={resolved.email}
            consume={async (email, code) => {
              await consumeOtp(email, code);
            }}
            onSignedIn={() => router.push("/practice")}
            onTokenTrouble={(code) => setPhase({ state: "trouble", code })}
          />
        ) : null}

        {resolved.state === "trouble" ? (
          <TokenTrouble code={resolved.code} requestHref="/otp" requestLabel="Send a new code" />
        ) : null}
      </AuthCard>
    </AuthShell>
  );
}
