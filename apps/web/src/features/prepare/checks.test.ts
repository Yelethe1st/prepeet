import { afterEach, describe, expect, it, vi } from "vitest";

import { realRunners } from "./checks";

/**
 * The device runners, against stubbed hardware.
 *
 * The properties worth holding are the promises the prepare screen makes
 * in words: the microphone is opened, measured and closed again with every
 * track stopped, and nothing is recorded. A refused microphone is a failed
 * check rather than a thrown error, because the screen must be able to say
 * what is wrong and offer a way on.
 */

const originalMediaDevices = navigator.mediaDevices;

afterEach(() => {
  vi.unstubAllGlobals();
  Object.defineProperty(navigator, "mediaDevices", {
    value: originalMediaDevices,
    configurable: true,
  });
});

/** A microphone whose samples sit at the given distance from silence. */
function stubMicrophone(amplitude: number) {
  const stopped: string[] = [];
  const track = { stop: () => stopped.push("stopped") };
  const stream = { getTracks: () => [track] } as unknown as MediaStream;
  Object.defineProperty(navigator, "mediaDevices", {
    value: { getUserMedia: vi.fn().mockResolvedValue(stream) },
    configurable: true,
  });
  const closed: string[] = [];
  vi.stubGlobal(
    "AudioContext",
    class {
      createAnalyser() {
        return {
          frequencyBinCount: 4,
          getByteTimeDomainData: (samples: Uint8Array) => {
            samples.fill(128 + amplitude);
          },
        };
      }
      createMediaStreamSource() {
        return { connect: () => undefined };
      }
      createOscillator() {
        return {
          frequency: { value: 0 },
          connect: (next: unknown) => next,
          start: () => undefined,
          stop: () => undefined,
        };
      }
      createGain() {
        return { gain: { value: 0 }, connect: (next: unknown) => next };
      }
      get destination() {
        return {};
      }
      close() {
        closed.push("closed");
        return Promise.resolve();
      }
    },
  );
  return { stopped, closed };
}

describe("the microphone check", () => {
  it("passes when it hears something, and closes everything it opened", async () => {
    const { stopped, closed } = stubMicrophone(40);

    await expect(realRunners.mic()).resolves.toBe("pass");

    expect(stopped).toHaveLength(1);
    expect(closed).toHaveLength(1);
  }, 10_000);

  it("fails quietly when the room is silent", async () => {
    const { stopped } = stubMicrophone(1);

    await expect(realRunners.mic()).resolves.toBe("fail");
    // Still closed: a failed check must not leave the microphone open.
    expect(stopped).toHaveLength(1);
  }, 10_000);

  it("fails rather than throwing when permission is refused", async () => {
    Object.defineProperty(navigator, "mediaDevices", {
      value: { getUserMedia: vi.fn().mockRejectedValue(new Error("denied")) },
      configurable: true,
    });

    await expect(realRunners.mic()).resolves.toBe("fail");
  });
});

describe("the speaker check", () => {
  it("plays a tone and closes the context afterwards", async () => {
    const { closed } = stubMicrophone(0);

    await realRunners.speaker();

    expect(closed).toHaveLength(1);
  }, 10_000);
});

describe("the network check", () => {
  it("passes a prompt response and fails a refused one", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true }));
    await expect(realRunners.net()).resolves.toBe("pass");

    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false }));
    await expect(realRunners.net()).resolves.toBe("fail");

    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));
    await expect(realRunners.net()).resolves.toBe("fail");
  });
});

describe("the browser check", () => {
  it("passes when the capabilities an interview needs are present", () => {
    stubMicrophone(0);
    vi.stubGlobal("WebSocket", class {});

    expect(realRunners.browser()).toBe("pass");
  });

  it("fails when the microphone API is missing entirely", () => {
    Object.defineProperty(navigator, "mediaDevices", {
      value: undefined,
      configurable: true,
    });

    expect(realRunners.browser()).toBe("fail");
  });
});
