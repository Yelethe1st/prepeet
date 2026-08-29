"use client";

import { useMutation } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { ApiError } from "@/lib/api/client";
import { completeOAuth } from "@/lib/auth/api";
import { Banner, ButtonLink, TextLink } from "@/shared/components";

/** After this long with no answer, say so rather than spin silently. */
const SLOW_AFTER_MS = 6000;

/**
 * Why a sign-in fails here, in the words the prototype uses.
 *
 * Kept as a list because all three are true of every failure and none of them
 * can be told apart from the outside: the server answers one refusal for a
 * forged state and a replayed one on purpose, so the screen cannot narrow it
 * either. Saying which three things it usually is beats claiming to know.
 */
const reasons = [
  "The sign-in was started in one tab and finished in another, so the one-time state value no longer matched.",
  "More than ten minutes passed on the provider's consent screen and the authorisation code expired.",
  "The callback link was copied and opened somewhere else. Codes are bound to the browser that started them.",
];

/**
 * The screen a provider redirects back to: IAM-08, from oauth-callback.html.
 *
 * One recorded deviation, and it is the whole design of the screen. The
 * prototype shows three stages with per-stage timings, ticking over as they
 * complete. There is one request here, and there is no way to know which part
 * of it the server is in. Rendering three stages that advance on a timer would
 * be an animation pretending to be telemetry, on the screen where somebody is
 * waiting to find out whether they are signed in. It shows what is true: this
 * is in progress, and after six seconds, that it is taking longer than usual.
 *
 * A failure never leaves somebody stranded. The fifth criterion asks that a
 * provider failure land on a screen naming what happened and offering email
 * and password, which is what the error state does, with the reference to
 * quote and the sign-in link beneath it.
 */
export function OAuthCallback({
  provider,
  providerLabel,
  state,
  code,
  providerError,
  onSignedIn,
}: {
  provider: string;
  providerLabel: string;
  /** The anti-forgery value the provider handed back, or empty if it did not. */
  state: string;
  code: string;
  /** What the provider itself reported, when it declined rather than redirected. */
  providerError: string;
  onSignedIn: (destination: string) => void;
}) {
  const [slow, setSlow] = useState(false);

  const finish = useMutation({
    mutationFn: () => completeOAuth(provider, state, code),
    onSuccess: () => onSignedIn("/practice"),
  });

  // Fired once, on arrival. The provider has already redirected; there is
  // nothing to ask the person and nothing for them to press.
  const { mutate } = finish;
  useEffect(() => {
    if (providerError !== "" || state === "" || code === "") return;
    mutate();
  }, [mutate, providerError, state, code]);

  useEffect(() => {
    if (!finish.isPending) return;
    const timer = setTimeout(() => setSlow(true), SLOW_AFTER_MS);
    return () => clearTimeout(timer);
  }, [finish.isPending]);

  const missing = state === "" || code === "";
  const failed = providerError !== "" || missing || finish.isError;

  if (failed) {
    const failure = finish.error instanceof ApiError ? finish.error : undefined;
    return (
      <div className="mx-auto w-full max-w-[420px] py-8">
        <h1 className="font-display text-2xl tracking-[-0.02em]">
          We could not finish that sign-in
        </h1>

        <Banner tone="danger">
          <strong>
            {providerError === ""
              ? (failure?.message ??
                "Something went wrong between your provider and Prepeet.")
              : `${providerLabel} did not complete the sign-in.`}
          </strong>
          {/*
            fg-2, not fg-3. The muted foreground on the danger surface measures
            4.48:1 in the light theme, under the 4.5 it needs, which the
            contrast sweep caught and looking at it would not have.
          */}
          <p className="mt-1 text-xs text-fg-2">
            No account details were exchanged. Starting again from the sign-in
            page fixes almost every case.
          </p>
        </Banner>

        <div className="mt-6 rounded-md border border-border bg-surface-2 p-4">
          <h2 className="mb-2 text-sm font-semibold">Why this happens</h2>
          <ul className="flex flex-col gap-2 text-sm leading-[1.55] text-fg-2">
            {reasons.map((reason) => (
              <li key={reason}>{reason}</li>
            ))}
          </ul>
        </div>

        <div className="mt-6">
          <ButtonLink href="/login" block>
            Back to sign in
          </ButtonLink>
        </div>

        <p className="mt-4 text-xs text-fg-3">
          {failure?.requestId === undefined ? null : (
            <>
              Reference <span className="font-mono">{failure.requestId}</span>{" "}
              ·{" "}
            </>
          )}
          <TextLink href="/login">
            Sign in with your email and password
          </TextLink>
        </p>
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-[420px] py-8 text-center">
      <h1 className="font-display text-2xl tracking-[-0.02em]">
        Completing sign-in with {providerLabel}…
      </h1>
      <p className="mt-2 text-sm leading-[1.55] text-fg-2">
        Do not close this tab. You will be taken on automatically.
      </p>

      {/*
        Announced, and only once it is true. A live region that says "working"
        from the first frame is a region that has said nothing by the time it
        matters.
      */}
      <p className="mt-6 text-sm text-fg-2" role="status" aria-live="polite">
        {slow
          ? "This is taking longer than usual. We are still trying, and you do not need to do anything."
          : "Verifying the response from your provider."}
      </p>

      <p className="mt-8 text-xs text-fg-3">
        <TextLink href="/login">Cancel and return to sign in</TextLink>
      </p>
    </div>
  );
}
