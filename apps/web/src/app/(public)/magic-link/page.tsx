"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { Suspense } from "react";

import { AuthCard } from "@/features/auth/AuthCard";
import { AuthShell } from "@/features/auth/AuthShell";
import { RequestEmailForm } from "@/features/auth/RequestEmailForm";
import { rememberSentEmail } from "@/features/auth/sentEmail";
import { TokenConsumer } from "@/features/auth/TokenConsumer";
import { Banner, ButtonLink } from "@/shared/components";
import { consumeMagicLink, requestTokenEmail } from "@/lib/auth/api";

/**
 * The magic-link route, from screens/magic-link.html.
 *
 * Two screens share the path, split by whether a token arrived: with one,
 * this is the landing from the email and the token is consumed on arrival;
 * without one, it is the request form the sign-in page links to. The
 * prototype only drew the landing; the request half reuses the same form as
 * recovery, because the server treats the two identically.
 *
 * Success keeps the person here rather than redirecting unasked. The session
 * cookies are already set; the button hands them the navigation, which is the
 * prototype's "go to your dashboard" moment.
 */
function MagicLink() {
  const token = useSearchParams().get("token") ?? "";
  const router = useRouter();

  if (!token) {
    return (
      <RequestEmailForm
        kind="magic_link"
        request={requestTokenEmail}
        onSent={(email) => {
          rememberSentEmail({ kind: "magic_link", email });
          router.push("/check-email");
        }}
      />
    );
  }

  return (
    <TokenConsumer
      token={token}
      consume={async (presented) => {
        await consumeMagicLink(presented);
      }}
      checking="Checking the link… keep this tab open for a moment."
      requestHref="/magic-link"
      requestLabel="Send a new sign-in link"
      done={
        <div>
          <Banner tone="success">
            You are signed in. This link has now been used up and will not work again.
          </Banner>
          <div className="mt-5">
            <ButtonLink href="/practice" variant="primary">
              Go to your dashboard
            </ButtonLink>
          </div>
        </div>
      }
    />
  );
}

export default function MagicLinkPage() {
  return (
    <AuthShell>
      <AuthCard title="Sign in by email">
        <Suspense fallback={null}>
          <MagicLink />
        </Suspense>
      </AuthCard>
    </AuthShell>
  );
}
