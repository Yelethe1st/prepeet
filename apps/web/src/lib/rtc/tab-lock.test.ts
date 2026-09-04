import { afterEach, describe, expect, it, vi } from "vitest";

import { claimLiveTab, type ChannelLike } from "./tab-lock";

/**
 * RTC-03's duplicate tab rule: one session, one live tab. The holder
 * answers for the session; a newcomer that hears an answer refuses to go
 * live; a browser with no channel at all grants rather than locking a
 * person out of their own interview.
 */

/** An in-memory broadcast bus: every channel with the same name hears it. */
function bus() {
  const channels = new Map<string, Set<FakeChannel>>();

  class FakeChannel implements ChannelLike {
    onmessage: ((event: { data: unknown }) => void) | null = null;
    closed = false;

    constructor(private readonly name: string) {
      const peers = channels.get(name) ?? new Set();
      peers.add(this);
      channels.set(name, peers);
    }

    postMessage(message: unknown): void {
      for (const peer of channels.get(this.name) ?? []) {
        if (peer !== this && !peer.closed) {
          peer.onmessage?.({ data: message });
        }
      }
    }

    close(): void {
      this.closed = true;
      channels.get(this.name)?.delete(this);
    }
  }

  return { create: (name: string) => new FakeChannel(name) };
}

/** Runs claims with a manual clock so silence is deterministic. */
function claimOn(create: (name: string) => ChannelLike) {
  const timers: (() => void)[] = [];
  const promise = (sessionId: string) =>
    claimLiveTab(sessionId, {
      createChannel: create,
      schedule: (run) => {
        timers.push(run);
      },
    });
  return { promise, elapse: () => timers.splice(0).forEach((run) => run()) };
}

describe("claiming the live tab", () => {
  it("grants on silence and then answers for the session", async () => {
    const { create } = bus();
    const first = claimOn(create);

    const claiming = first.promise("ses-1");
    first.elapse();
    const claim = await claiming;
    expect(claim.granted).toBe(true);

    // A second tab asks, hears the holder, and refuses without waiting out
    // any timer: the answer is what settles it.
    const second = claimOn(create);
    const refused = await second.promise("ses-1");
    expect(refused.granted).toBe(false);
  });

  it("scopes the claim to the session", async () => {
    const { create } = bus();
    const first = claimOn(create);
    const holding = first.promise("ses-1");
    first.elapse();
    await holding;

    // A different session's tab hears nothing and takes its own claim.
    const other = claimOn(create);
    const claiming = other.promise("ses-2");
    other.elapse();
    const claim = await claiming;
    expect(claim.granted).toBe(true);
  });

  it("release stops answering, so the next tab can claim", async () => {
    const { create } = bus();
    const first = claimOn(create);
    const holding = first.promise("ses-1");
    first.elapse();
    const claim = await holding;

    claim.release();
    claim.release(); // idempotent

    const second = claimOn(create);
    const claiming = second.promise("ses-1");
    second.elapse();
    const succeeded = await claiming;
    expect(succeeded.granted).toBe(true);
  });

  it("grants when the browser has no broadcast channel at all", async () => {
    vi.stubGlobal("BroadcastChannel", undefined);

    const claim = await claimLiveTab("ses-1");

    expect(claim.granted).toBe(true);
    claim.release();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });
});
