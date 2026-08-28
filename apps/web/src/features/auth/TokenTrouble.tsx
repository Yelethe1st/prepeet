"use client";

import { ButtonLink, TextLink } from "@/shared/components";

/**
 * The screen for a token that did not work, ported from auth-expired.html.
 *
 * One component for every dead state, because the prototype's promise is that
 * each gets its own explanation rather than one generic failure — and the
 * cheapest way to keep six states honest is to write their words side by side
 * where a gap is visible.
 *
 * Every state leads with what did NOT happen. A person clicking a dead reset
 * link is worried their account changed; "nothing has changed" is the answer
 * they came for, before any instruction.
 */

/** The token outcome codes the server distinguishes, plus the code cap. */
export type TokenTroubleCode =
  | "TOKEN_EXPIRED"
  | "TOKEN_USED"
  | "TOKEN_SUPERSEDED"
  | "TOKEN_INVALID"
  | "CODE_ATTEMPTS_EXHAUSTED";

interface TroubleCopy {
  headline: string;
  body: string;
  /** Whether asking for a fresh email is the way forward. */
  resend: boolean;
}

const troubles: Record<TokenTroubleCode, TroubleCopy> = {
  TOKEN_EXPIRED: {
    headline: "This link has expired",
    body: "Nothing has changed on your account, and nothing was signed in. Request a fresh link and it will be in your inbox within a minute.",
    resend: true,
  },
  TOKEN_USED: {
    headline: "This link has already been used",
    body: "It did its work the first time, so nothing further is needed. If that was not you, request a fresh link now.",
    resend: true,
  },
  TOKEN_SUPERSEDED: {
    headline: "A newer email has replaced this link",
    body: "Requesting a new link invalidates the previous one immediately, so only the newest email works. Check your inbox for the most recent one.",
    resend: false,
  },
  TOKEN_INVALID: {
    headline: "This link is not valid",
    body: "Nothing has changed on your account. Check the link was copied completely from the email, or request a fresh one.",
    resend: true,
  },
  CODE_ATTEMPTS_EXHAUSTED: {
    headline: "Too many incorrect codes",
    body: "That code no longer works. Nothing has changed on your account. Request a fresh code and use the newest email.",
    resend: true,
  },
};

interface TokenTroubleProps {
  code: TokenTroubleCode;
  /** Where "send a new one" goes: the request form for this flow. */
  requestHref: string;
  /** What the fresh email would be, for the button's label. */
  requestLabel: string;
}

export function TokenTrouble({
  code,
  requestHref,
  requestLabel,
}: TokenTroubleProps) {
  const trouble = troubles[code];

  return (
    <div>
      <h1 className="font-display text-xl">{trouble.headline}</h1>
      <p className="mt-3 text-sm text-fg-2">{trouble.body}</p>

      <div className="mt-6 flex flex-wrap items-center gap-3">
        {trouble.resend ? (
          <ButtonLink href={requestHref} variant="primary">
            {requestLabel}
          </ButtonLink>
        ) : null}
        <TextLink href="/login">Back to sign in</TextLink>
      </div>

      <section className="mt-10 border-t border-border pt-5">
        <h2 className="text-sm font-semibold">Why Prepeet links expire</h2>
        <p className="mt-2 text-sm text-fg-2">
          A short window limits the damage of a leaked email. Forwarded
          messages, shared mailboxes and printed handovers are all common in the
          workplaces we serve. An hour-old link is a much smaller problem than a
          permanent one.
        </p>
      </section>
    </div>
  );
}
