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
 * While the list is loading it draws placeholders, as login.html does. A row of
 * buttons that appears a beat after somebody has started typing their password
 * moves the form under their hands, and a person who never saw the options
 * cannot know they were offered. The placeholders reserve the space and say
 * what is being waited for.
 */

/**
 * The brand mark beside each label, as the prototype draws it.
 *
 * Colours are the providers' own, which is what makes a row of sign-in buttons
 * scannable at a glance rather than three identical rectangles. Google's mark
 * carries a ring because its brand white would otherwise vanish into the
 * button beneath it.
 *
 * A provider this build has never heard of still gets a mark, from the first
 * letter of its label. A hardcoded list that silently omitted one would be the
 * opposite of drawing the buttons from the server's answer.
 */
const BRAND_MARKS: Record<
  string,
  { letter: string; style: React.CSSProperties }
> = {
  google: {
    letter: "G",
    style: {
      background: "#FFFFFF",
      color: "#3C4043",
      boxShadow: "inset 0 0 0 1px #DADCE0",
    },
  },
  microsoft: {
    letter: "M",
    style: { background: "#0067B8", color: "#FFFFFF" },
  },
};

function markFor(provider: { id: string; label: string }) {
  return (
    BRAND_MARKS[provider.id] ?? {
      letter: (provider.label.trim()[0] ?? "?").toUpperCase(),
      style: {
        background: "var(--color-surface-2)",
        color: "var(--color-fg-2)",
      },
    }
  );
}
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

  // Placeholders only until the first attempt has failed, not for as long as
  // the query is pending. react-query keeps retrying, so isPending alone stays
  // true through every retry and a person whose API is unreachable would watch
  // skeleton buttons for as long as they cared to look.
  //
  // A provider list that is not answering is one we should stop promising. The
  // form beneath is fully usable, and no buttons is the honest state.
  if (providers.isPending && providers.failureCount === 0) {
    return (
      <div className="mb-6" aria-busy="true">
        <p className="sr-only">Checking which sign-in options are available…</p>
        <div className="flex flex-col gap-2">
          {[0, 1].map((slot) => (
            <div
              key={slot}
              className="h-10 rounded-md bg-surface-2"
              aria-hidden="true"
            />
          ))}
        </div>
      </div>
    );
  }

  const configured = providers.data?.providers ?? [];
  // Once the answer is known and empty, the placeholders go entirely. Leaving
  // them up would be a promise of something that is not coming.
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
            <span
              data-provider-mark
              aria-hidden="true"
              className="grid h-[18px] w-[18px] place-items-center rounded-[4px] text-[10px] font-extrabold"
              style={markFor(provider).style}
            >
              {markFor(provider).letter}
            </span>
            <span>Continue with {provider.label}</span>
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
