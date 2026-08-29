"use client";

import { useParams, useRouter, useSearchParams } from "next/navigation";

import { AuthShell } from "@/features/auth/AuthShell";
import { OAuthCallback } from "@/features/auth/OAuthCallback";

/** What each provider is called, for a sentence somebody reads while waiting. */
const labels: Record<string, string> = {
  google: "Google",
  microsoft: "Microsoft",
};

/**
 * Where a provider redirects back to, ported from screens/oauth-callback.html.
 *
 * The route owns reading the query and where to go next; everything a person
 * sees is in OAuthCallback, which is why that has the tests.
 *
 * The provider is in the path rather than the query because it is part of the
 * redirect URI registered with each provider, and a registered URI that varies
 * by query string is one more thing to get wrong in two places.
 */
export default function OAuthCallbackPage() {
  const router = useRouter();
  const params = useParams<{ provider: string }>();
  const query = useSearchParams();

  const provider = params.provider;

  return (
    <AuthShell>
      <OAuthCallback
        provider={provider}
        providerLabel={labels[provider] ?? provider}
        state={query.get("state") ?? ""}
        code={query.get("code") ?? ""}
        // A provider that declines redirects with error rather than code, and
        // says why in the query. It is shown as a failure rather than left to
        // become "no code", which would be true and unhelpful.
        providerError={query.get("error") ?? ""}
        onSignedIn={(destination) => router.push(destination)}
      />
    </AuthShell>
  );
}
