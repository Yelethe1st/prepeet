import { afterEach, describe, expect, it } from "vitest";

import { consumeGrant, stashGrant, type StoredGrant } from "./grant";
import { resetGrantMemoryForTests } from "./grant";

/** The hand-off: one navigation, one use, and expiry honoured on read. */

const grant: StoredGrant = {
  sessionId: "ses-1",
  url: "wss://rtc.test",
  room: "ses-1",
  token: "tok",
  expiresAt: new Date(Date.now() + 60_000).toISOString(),
};

afterEach(() => {
  sessionStorage.clear();
  resetGrantMemoryForTests();
});

describe("the grant hand-off", () => {
  it("leaves nothing in storage after the first read, so a refresh cannot reuse it", () => {
    stashGrant(grant);

    expect(consumeGrant("ses-1")).toEqual(grant);
    // The same page load may read again (development StrictMode runs
    // effects twice); what one-use protects against is the token surviving
    // in storage, and it does not.
    expect(sessionStorage.getItem("prepeet.grant.ses-1")).toBeNull();
  });

  it("an expired grant reads as absent", () => {
    stashGrant({
      ...grant,
      expiresAt: new Date(Date.now() - 1000).toISOString(),
    });

    expect(consumeGrant("ses-1")).toBeNull();
  });

  it("is scoped per session", () => {
    stashGrant(grant);

    expect(consumeGrant("ses-2")).toBeNull();
    expect(consumeGrant("ses-1")).toEqual(grant);
  });

  it("an expired remembered grant is not resurrected", () => {
    stashGrant({
      ...grant,
      sessionId: "ses-3",
      expiresAt: new Date(Date.now() + 5).toISOString(),
    });
    expect(consumeGrant("ses-3")).not.toBeNull();

    // Past expiry, the in-memory memory refuses too.
    const later = Date.now() + 60_000;
    const realNow = Date.now;
    Date.now = () => later;
    try {
      expect(consumeGrant("ses-3")).toBeNull();
    } finally {
      Date.now = realNow;
    }
  });
});
