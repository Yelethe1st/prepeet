import { browserSupport, type CheckStatus } from "./gate";

/**
 * The device check runners: what actually talks to the hardware, kept apart
 * from the screen so the screen's behaviour is testable with fakes and the
 * hardware access lives in one auditable place.
 *
 * Nothing here records anything. The microphone check opens the microphone,
 * measures level for a few seconds and closes it again; no audio leaves the
 * browser, which is the promise the prepare screen makes in words.
 */

export interface CheckRunners {
  /** Opens the microphone, listens briefly, closes it. */
  mic: () => Promise<CheckStatus>;
  /** Plays a short tone; the person confirms they heard it. */
  speaker: () => Promise<void>;
  /** Times a small request to the API. */
  net: () => Promise<CheckStatus>;
  /** Detects the capabilities an interview needs. Synchronous by nature. */
  browser: () => CheckStatus;
}

/** How long the microphone is listened to before deciding. */
const micListenMs = 3000;
/** The peak level that counts as "we heard you". */
const micThreshold = 0.04;
/** A connection slower than this to our own API will struggle with audio. */
const netBudgetMs = 1500;

export const realRunners: CheckRunners = {
  async mic() {
    let stream: MediaStream;
    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    } catch {
      return "fail";
    }
    try {
      const context = new AudioContext();
      const analyser = context.createAnalyser();
      context.createMediaStreamSource(stream).connect(analyser);
      const samples = new Uint8Array(analyser.frequencyBinCount);

      const deadline = Date.now() + micListenMs;
      let peak = 0;
      while (Date.now() < deadline) {
        analyser.getByteTimeDomainData(samples);
        for (const sample of samples) {
          peak = Math.max(peak, Math.abs(sample - 128) / 128);
        }
        await new Promise((resolve) => setTimeout(resolve, 100));
      }
      await context.close();
      return peak >= micThreshold ? "pass" : "fail";
    } finally {
      for (const track of stream.getTracks()) {
        track.stop();
      }
    }
  },

  async speaker() {
    const context = new AudioContext();
    const oscillator = context.createOscillator();
    const gain = context.createGain();
    gain.gain.value = 0.2;
    oscillator.frequency.value = 440;
    oscillator.connect(gain).connect(context.destination);
    oscillator.start();
    await new Promise((resolve) => setTimeout(resolve, 900));
    oscillator.stop();
    await context.close();
  },

  async net() {
    const started = performance.now();
    try {
      const response = await fetch("/livez", { cache: "no-store" });
      if (!response.ok) {
        return "fail";
      }
    } catch {
      return "fail";
    }
    return performance.now() - started <= netBudgetMs ? "pass" : "fail";
  },

  browser() {
    return browserSupport({
      hasGetUserMedia:
        typeof navigator !== "undefined" &&
        !!navigator.mediaDevices?.getUserMedia,
      hasAudioContext: typeof AudioContext !== "undefined",
      hasWebSocket: typeof WebSocket !== "undefined",
    })
      ? "pass"
      : "fail";
  },
};
