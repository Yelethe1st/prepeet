/**
 * The room grant's hand-off between the prepare screen and the live route.
 *
 * The grant is short-lived and bearer, so it travels in sessionStorage for
 * exactly one navigation and is consumed on read: a refresh of the live
 * page after joining finds nothing and lands on the named recovery path
 * rather than silently reusing a token that may have expired. Nothing here
 * is durable and nothing survives the tab.
 */

export interface StoredGrant {
  sessionId: string;
  url: string;
  room: string;
  token: string;
  expiresAt: string;
}

const key = (sessionId: string): string => `prepeet.grant.${sessionId}`;

// Consumed grants, kept for this page load only. React's development
// StrictMode runs effects twice; without this, the second run would find
// the storage already consumed and land on recovery instead of joining.
// Memory dies with the tab, so the one-use property holds where it
// matters: across navigations and refreshes.
const consumedThisLoad = new Map<string, StoredGrant>();

export function stashGrant(grant: StoredGrant): void {
  try {
    sessionStorage.setItem(key(grant.sessionId), JSON.stringify(grant));
  } catch {
    // Storage being unavailable only costs the hand-off; the live route
    // will show its named recovery instead of joining.
  }
}

/** Reads and removes the grant: one navigation, one use. */
export function consumeGrant(sessionId: string): StoredGrant | null {
  try {
    const raw = sessionStorage.getItem(key(sessionId));
    sessionStorage.removeItem(key(sessionId));
    if (!raw) {
      const remembered = consumedThisLoad.get(sessionId);
      if (remembered && new Date(remembered.expiresAt).getTime() > Date.now()) {
        return remembered;
      }
      return null;
    }
    const grant = JSON.parse(raw) as StoredGrant;
    if (new Date(grant.expiresAt).getTime() <= Date.now()) {
      return null;
    }
    consumedThisLoad.set(sessionId, grant);
    return grant;
  } catch {
    return null;
  }
}

/**
 * Forgets the per-load memory. Tests share one module instance across many
 * simulated page loads and call this between them; the application never
 * needs it, because real memory dies with the tab.
 */
export function resetGrantMemoryForTests(): void {
  consumedThisLoad.clear();
}
