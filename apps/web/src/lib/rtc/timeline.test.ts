import { describe, expect, it, vi } from "vitest";

import { Timeline, type AckView, type WireEvent } from "./timeline";

/**
 * RTC-03's resend buffer: the server's record is the record, and the buffer
 * holds only what is not yet confirmed. Identity survives retries and
 * takeovers, order is never invented, and a refusal is dropped by name
 * rather than resent forever.
 */

function ackAll(epoch: number, events: WireEvent[]): AckView {
  return {
    accepted_sequence: Math.max(0, ...events.map((event) => event.sequence)),
    outcomes: events.map((event) => ({
      event_id: event.event_id,
      status: "accepted" as const,
    })),
  };
}

function timelineWith(
  post: (epoch: number, events: WireEvent[]) => Promise<AckView>,
  onRefused?: (event: WireEvent, reason: string) => void,
) {
  let id = 0;
  return new Timeline({
    post,
    epoch: 1,
    onRefused,
    makeId: () => `evt-${++id}`,
    now: () => new Date("2026-09-04T12:00:00Z"),
  });
}

describe("recording and confirmation", () => {
  it("numbers events in order and clears what the server confirms", async () => {
    const posts: { epoch: number; events: WireEvent[] }[] = [];
    const timeline = timelineWith((epoch, events) => {
      posts.push({ epoch, events });
      return Promise.resolve(ackAll(epoch, events));
    });

    timeline.record("connection.established");
    timeline.record("preference.captions", { enabled: true });
    await timeline.flush();

    expect(posts).toHaveLength(1);
    expect(posts[0]?.epoch).toBe(1);
    expect(posts[0]?.events.map((event) => event.sequence)).toEqual([1, 2]);
    expect(posts[0]?.events[1]?.payload).toEqual({ enabled: true });
    expect(timeline.pending).toBe(0);
  });

  it("keeps the whole tail when the post fails, and resends it with the same identities", async () => {
    const posts: WireEvent[][] = [];
    let fail = true;
    const timeline = timelineWith((epoch, events) => {
      posts.push(events);
      if (fail) {
        return Promise.reject(new Error("offline"));
      }
      return Promise.resolve(ackAll(epoch, events));
    });

    timeline.record("connection.established");
    await expect(timeline.flush()).rejects.toThrow("offline");
    expect(timeline.pending).toBe(1);

    fail = false;
    await timeline.flush();

    // Same event, same identity, same slot: the retry converges instead of
    // doubling, because the id is what the server deduplicates on.
    expect(posts[1]).toEqual(posts[0]);
    expect(timeline.pending).toBe(0);
  });

  it("keeps an event the acknowledgment left unconfirmed", async () => {
    const timeline = timelineWith((_epoch, events) =>
      Promise.resolve({
        // The server confirmed only the first: the second's outcome is
        // missing (a partial answer) and the cursor stops below it.
        accepted_sequence: 1,
        outcomes: [
          { event_id: events[0]?.event_id ?? "", status: "accepted" as const },
        ],
      }),
    );

    timeline.record("connection.established");
    timeline.record("turn.boundary");
    await timeline.flush();

    expect(timeline.pending).toBe(1);
  });

  it("drops a refused event by name instead of resending it forever", async () => {
    const refusals: string[] = [];
    const timeline = timelineWith(
      (_epoch, events) =>
        Promise.resolve({
          accepted_sequence: 0,
          outcomes: events.map((event, index) => ({
            event_id: event.event_id,
            status: index === 0 ? ("refused" as const) : ("accepted" as const),
            reason: index === 0 ? "EVENT_TYPE_UNKNOWN" : undefined,
          })),
        }),
      (event, reason) => refusals.push(`${event.type}:${reason}`),
    );

    timeline.record("something.unknown");
    timeline.record("turn.boundary");
    await timeline.flush();

    expect(refusals).toEqual(["something.unknown:EVENT_TYPE_UNKNOWN"]);
    // The refusal is gone; the accepted one is confirmed; nothing lingers.
    expect(timeline.pending).toBe(0);
  });

  it("runs one batch at a time so two flushes cannot interleave", async () => {
    let inFlight = 0;
    let peak = 0;
    const timeline = timelineWith(async (epoch, events) => {
      inFlight += 1;
      peak = Math.max(peak, inFlight);
      await new Promise((resolve) => setTimeout(resolve, 0));
      inFlight -= 1;
      return ackAll(epoch, events);
    });

    timeline.record("connection.established");
    const first = timeline.flush();
    timeline.record("turn.boundary");
    const second = timeline.flush();
    await Promise.all([first, second]);
    // The second event may have missed the first batch; the follow-up flush
    // the buffer schedules for itself delivers it.
    await vi.waitFor(() => expect(timeline.pending).toBe(0));

    expect(peak).toBe(1);
  });

  it("flushing an empty buffer sends nothing", async () => {
    const posts: WireEvent[][] = [];
    const timeline = timelineWith((epoch, events) => {
      posts.push(events);
      return Promise.resolve(ackAll(epoch, events));
    });

    await timeline.flush();

    expect(posts).toHaveLength(0);
  });
});

describe("rebasing into a new epoch after a resume", () => {
  it("confirms what the recovery cursor covers and renumbers the survivors from one", async () => {
    const posts: { epoch: number; events: WireEvent[] }[] = [];
    const timeline = timelineWith((epoch, events) => {
      posts.push({ epoch, events });
      return Promise.reject(new Error("offline"));
    });

    timeline.record("connection.established"); // seq 1, held by the server
    timeline.record("turn.boundary"); // seq 2, held by the server
    timeline.record("preference.captions"); // seq 3, lost with the connection
    timeline.record("turn.boundary"); // seq 4, lost with the connection
    await expect(timeline.flush()).rejects.toThrow("offline");

    // The resume said epoch 1 durably holds through sequence 2.
    timeline.rebase(2, 2);

    expect(timeline.currentEpoch).toBe(2);
    expect(timeline.pending).toBe(2);

    // The survivors renumber from one, in recorded order, keeping their
    // identities so anything that did land converges to a duplicate.
    await expect(timeline.flush()).rejects.toThrow("offline");
    const resent = posts[1];
    expect(resent?.epoch).toBe(2);
    expect(resent?.events.map((event) => event.sequence)).toEqual([1, 2]);
    expect(resent?.events.map((event) => event.event_id)).toEqual([
      "evt-3",
      "evt-4",
    ]);
  });

  it("numbers new events after the rebased survivors", async () => {
    const posts: { epoch: number; events: WireEvent[] }[] = [];
    const timeline = timelineWith((epoch, events) => {
      posts.push({ epoch, events });
      return Promise.resolve(ackAll(epoch, events));
    });

    timeline.record("turn.boundary");
    timeline.rebase(3, 0);
    timeline.record("connection.established");
    await timeline.flush();

    expect(posts[0]?.epoch).toBe(3);
    expect(posts[0]?.events.map((event) => event.sequence)).toEqual([1, 2]);
  });
});
