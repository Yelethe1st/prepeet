import { describe, expect, it } from "vitest";

import { browserSupport, startBlocker, type Checks } from "./gate";

/**
 * The start gate's rules, pure. SES-03's first two boxes live here: what
 * blocks start, and which single problem the blocked state names - because
 * a person fixes one thing at a time, and the message and the focus target
 * must agree about which thing that is.
 */

const allPassed: Checks = {
  mic: "pass",
  speaker: "pass",
  net: "pass",
  browser: "pass",
};

describe("startBlocker", () => {
  it("clears only when the microphone and browser pass and consent is given", () => {
    expect(startBlocker(allPassed, true)).toBeNull();
  });

  it("blocks on the microphone until it passes, whatever else is fine", () => {
    const blocked = startBlocker({ ...allPassed, mic: "pending" }, true);

    expect(blocked?.target).toBe("mic");
    expect(blocked?.message).toMatch(/microphone check/i);
  });

  it("names a failed microphone differently from an unrun one", () => {
    expect(startBlocker({ ...allPassed, mic: "fail" }, true)?.message).toMatch(
      /until the microphone check passes/i,
    );
    expect(
      startBlocker({ ...allPassed, mic: "pending" }, true)?.message,
    ).toMatch(/run the microphone check/i);
  });

  it("an unsupported browser outranks everything, because nothing else can be fixed in it", () => {
    const blocked = startBlocker(
      { ...allPassed, browser: "fail", mic: "fail" },
      false,
    );

    expect(blocked?.target).toBe("browser");
    expect(blocked?.message).toMatch(/supported browser/i);
  });

  it("blocks on consent last, and says nothing is recorded until it is given", () => {
    const blocked = startBlocker(allPassed, false);

    expect(blocked?.target).toBe("consent");
    expect(blocked?.message).toMatch(/agree to recording/i);
    expect(blocked?.message).toMatch(/nothing is recorded/i);
  });

  it("never requires the recommended checks", () => {
    expect(
      startBlocker({ ...allPassed, speaker: "pending", net: "fail" }, true),
    ).toBeNull();
  });
});

describe("browserSupport", () => {
  it("requires microphone capture, audio playback and websockets", () => {
    expect(
      browserSupport({
        hasGetUserMedia: true,
        hasAudioContext: true,
        hasWebSocket: true,
      }),
    ).toBe(true);
    expect(
      browserSupport({
        hasGetUserMedia: false,
        hasAudioContext: true,
        hasWebSocket: true,
      }),
    ).toBe(false);
    expect(
      browserSupport({
        hasGetUserMedia: true,
        hasAudioContext: false,
        hasWebSocket: true,
      }),
    ).toBe(false);
    expect(
      browserSupport({
        hasGetUserMedia: true,
        hasAudioContext: true,
        hasWebSocket: false,
      }),
    ).toBe(false);
  });
});
