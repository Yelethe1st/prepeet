import { useSyncExternalStore } from "react";

import type { TokenEmailKind } from "@/lib/auth/api";

/**
 * What the check-email screen knows about the email that was just sent.
 *
 * Session storage rather than the URL, because the URL would put the address
 * into browser history and, on a hard reload, into a server log. Session
 * storage is per-tab and gone when the tab closes, which matches how long the
 * fact is useful.
 *
 * Everything is wrapped in try/catch because storage genuinely throws in some
 * contexts (private windows, blocked site data), and the screen must render
 * with generic copy rather than crash when it does.
 */

const KEY = "prepeet.sent-email";

/** SentEmail is the fact the check-email screen renders. */
export interface SentEmail {
  kind: TokenEmailKind;
  email: string;
}

/** rememberSentEmail records what was just sent, for the next screen. */
export function rememberSentEmail(sent: SentEmail): void {
  try {
    sessionStorage.setItem(KEY, JSON.stringify(sent));
  } catch {
    // The next screen falls back to generic copy.
  }
}

/** readSentEmail returns the last send, or null when there is none to show. */
export function readSentEmail(): SentEmail | null {
  try {
    const raw = sessionStorage.getItem(KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<SentEmail>;
    if (typeof parsed.email !== "string" || typeof parsed.kind !== "string")
      return null;
    return { email: parsed.email, kind: parsed.kind };
  } catch {
    return null;
  }
}

/**
 * maskEmail shows enough of an address to recognise it and no more.
 *
 * The prototype's shape: first and last character of the local part with the
 * middle dotted out. The screen it appears on can be read over a shoulder in
 * a way the person's own inbox cannot.
 */
export function maskEmail(email: string): string {
  const at = email.indexOf("@");
  if (at < 1) return email;
  const local = email.slice(0, at);
  if (local.length <= 2) return `${local[0]}•@${email.slice(at + 1)}`;
  return `${local[0]}${"•".repeat(Math.min(local.length - 2, 9))}${local[local.length - 1]}@${email.slice(at + 1)}`;
}

/**
 * The raw value last parsed, keyed by the string it was parsed from, because
 * useSyncExternalStore compares snapshots by identity and a fresh object per
 * read would render forever.
 */
let cached: { raw: string | null; value: SentEmail | null } = {
  raw: null,
  value: null,
};

function snapshot(): SentEmail | null {
  let raw: string | null = null;
  try {
    raw = sessionStorage.getItem(KEY);
  } catch {
    raw = null;
  }
  if (raw !== cached.raw) {
    cached = { raw, value: readSentEmail() };
  }
  return cached.value;
}

/**
 * useSentEmail reads the last send, hydration-safely.
 *
 * The server snapshot is null because session storage does not exist there,
 * and the client value arrives in the post-hydration render, which is the
 * mechanism this hook exists for: reading it during render would make the
 * first client render differ from the server's, and doing it in an effect is
 * a state set the compiler rightly refuses.
 */
export function useSentEmail(): SentEmail | null {
  return useSyncExternalStore(
    () => () => {},
    snapshot,
    () => null,
  );
}
