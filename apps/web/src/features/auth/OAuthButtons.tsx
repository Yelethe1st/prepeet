"use client";

import { useMutation, useQuery } from "@tanstack/react-query";

import { listOAuthProviders, startOAuth } from "@/lib/auth/api";
import { Button } from "@/shared/components";

/**
 * A button per configured sign-in provider: IAM-08.
 *
 * Drawn from the server's list rather than hard-coded, which is the point of
 * the sixth criterion: adding a provider is configuration and a deployment
 * rather than a release here as well as in the service.
 *
 * A deployment with none configured renders nothing at all, not an empty
 * heading or a divider with nothing above it. The prototype's "or sign in with
 * email" rule only exists when there is something to be an alternative to.
 *
 * While the list is loading it renders nothing either. A row of buttons that
 * appears a beat after the form has already been read is worse than one that
 * was never offered, and the form beneath is fully usable in the meantime.
 */
export function OAuthButtons({ redirectTo }: { redirectTo?: string }) {
  const providers = useQuery({
    queryKey: ["oauth-providers"],
    queryFn: listOAuthProviders,
    // The set changes when a deployment changes, not while somebody is
    // looking at the page.
    staleTime: 5 * 60 * 1000,
  });

  const begin = useMutation({
    mutationFn: (provider: string) => startOAuth(provider, redirectTo),
    onSuccess: (start) => {
      // A full navigation rather than a router push: the destination is the
      // provider's origin, which the application router cannot route to.
      window.location.assign(start.authorization_url);
    },
  });

  const configured = providers.data?.providers ?? [];
  if (configured.length === 0) return null;

  return (
    <div className="mb-6">
      <div className="flex flex-col gap-2">
        {configured.map((provider) => (
          <Button
            key={provider.id}
            variant="secondary"
            block
            busy={begin.isPending && begin.variables === provider.id}
            busyLabel={`Opening ${provider.label}…`}
            onClick={() => begin.mutate(provider.id)}
          >
            Continue with {provider.label}
          </Button>
        ))}
      </div>

      {begin.isError && (
        // Named, and not fatal: the form below still works, so this says what
        // failed rather than taking the screen over.
        <p className="mt-2 text-xs text-danger-fg" role="alert">
          That provider could not be reached. Sign in with your email and
          password below.
        </p>
      )}

      <p className="mt-6 flex items-center gap-3 text-2xs tracking-[0.08em] text-fg-3 uppercase">
        <span className="h-px flex-1 bg-border" />
        or sign in with email
        <span className="h-px flex-1 bg-border" />
      </p>
    </div>
  );
}
