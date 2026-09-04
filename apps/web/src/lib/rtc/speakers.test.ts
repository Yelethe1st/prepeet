import { describe, expect, it } from "vitest";

import { observeSpeakers, type SpeakerRoom } from "./speakers";

/**
 * The coarse speaking reading: the interviewer, the candidate, or nobody,
 * from the SFU's own voice-activity signal, loudest first when both talk.
 */

function fakeRoom() {
  const handlers = new Set<(speakers: { identity: string }[]) => void>();
  const room: SpeakerRoom = {
    on: (_event, handler) => handlers.add(handler),
    off: (_event, handler) => handlers.delete(handler),
  };
  return {
    room,
    emit: (speakers: { identity: string }[]) => {
      for (const handler of handlers) {
        handler(speakers);
      }
    },
    handlerCount: () => handlers.size,
  };
}

describe("observing who speaks", () => {
  it("names the candidate, the interviewer, and silence", () => {
    const { room, emit } = fakeRoom();
    const readings: (string | null)[] = [];
    observeSpeakers(room, "interviewer", (speaker) => readings.push(speaker));

    emit([{ identity: "user-1" }]);
    emit([{ identity: "interviewer" }]);
    emit([]);
    // Both talking: the SFU orders by loudness and the loudest decides.
    emit([{ identity: "interviewer" }, { identity: "user-1" }]);

    expect(readings).toEqual(["user", "ai", null, "ai"]);
  });

  it("unsubscribing stops the readings", () => {
    const { room, emit, handlerCount } = fakeRoom();
    const readings: (string | null)[] = [];
    const stop = observeSpeakers(room, "interviewer", (speaker) =>
      readings.push(speaker),
    );

    stop();
    emit([{ identity: "user-1" }]);

    expect(readings).toEqual([]);
    expect(handlerCount()).toBe(0);
  });
});
