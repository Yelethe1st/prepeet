"use client";

import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";

import { ApiError } from "@/lib/api/client";

import { TokenTrouble } from "./TokenTrouble";
import type { TokenTroubleCode } from "./TokenTrouble";

/**
 * Consumes a link token on arrival: verify-email.html and magic-link.html.
 *
 * Both screens are the same machine — checking, then done or trouble — with
 * different words and a different effect. The machine lives once so the two
 * cannot diverge in how they treat the states, which is where the prototype's
 * promise about distinct outcomes would quietly break.
 *
 * The consume fires exactly once per mount. A re-render must not present the
 * token again: the second presentation would be answered "already used" and
 * the person would be told their own click beat them.
 */

/** The token outcome codes, read from the error the server sent. */
const troubleCodes: ReadonlySet<string> = new Set([
  "TOKEN_INVALID",
  "TOKEN_EXPIRED",
  "TOKEN_USED",
  "TOKEN_SUPERSEDED",
]);

type Phase = { state: "checking" } | { state: "done" } | { state: "trouble"; code: TokenTroubleCode };

interface TokenConsumerProps {
  /** The token from the link, or empty when the URL carried none. */
  token: string;
  consume: (token: string) => Promise<void>;
  /** What the screen says while the token is being checked. */
  checking: string;
  /** What the screen shows once the token did its work. */
  done: ReactNode;
  /** Where a fresh email can be requested from. */
  requestHref: string;
  requestLabel: string;
}

export function TokenConsumer({
  token,
  consume,
  checking,
  done,
  requestHref,
  requestLabel,
}: TokenConsumerProps) {
  const [phase, setPhase] = useState<Phase>(
    token ? { state: "checking" } : { state: "trouble", code: "TOKEN_INVALID" },
  );
  const presented = useRef(false);

  useEffect(() => {
    if (!token || presented.current) return;
    presented.current = true;

    consume(token).then(
      () => setPhase({ state: "done" }),
      (error: unknown) => {
        if (error instanceof ApiError && troubleCodes.has(error.code)) {
          setPhase({ state: "trouble", code: error.code as TokenTroubleCode });
          return;
        }
        // A network failure is not a dead token; saying "invalid" would send
        // somebody to request a new link they do not need. INVALID is still
        // the closest screen we have for "could not check", and its copy leads
        // with nothing having changed, which remains true.
        setPhase({ state: "trouble", code: "TOKEN_INVALID" });
      },
    );
  }, [token, consume]);

  if (phase.state === "checking") {
    return (
      <p aria-live="polite" className="text-sm text-fg-2">
        {checking}
      </p>
    );
  }

  if (phase.state === "trouble") {
    return <TokenTrouble code={phase.code} requestHref={requestHref} requestLabel={requestLabel} />;
  }

  return <div aria-live="polite">{done}</div>;
}
