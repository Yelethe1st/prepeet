/**
 * The start gate's rules: SES-03's contract that start is disabled until
 * the microphone check passes and required consent is given, and that the
 * blocked state names exactly one problem - the one focus will move to.
 *
 * The microphone and browser are required because the interview cannot
 * happen without them; speaker and connection are strongly recommended
 * because it can, badly. Consent is last in the order deliberately: by the
 * time it is the blocker, everything mechanical is settled and the choice
 * gets the person's full attention.
 */

export type CheckStatus = "pending" | "testing" | "confirm" | "pass" | "fail";

export interface Checks {
  mic: CheckStatus;
  speaker: CheckStatus;
  net: CheckStatus;
  browser: CheckStatus;
}

export interface Blocked {
  /** Where "take me to what is missing" moves focus. */
  target: "browser" | "mic" | "consent";
  /** What the blocked state says, beside the disabled button. */
  message: string;
}

/** What blocks start, or null when nothing does. */
export function startBlocker(
  checks: Checks,
  requiredConsent: boolean,
): Blocked | null {
  if (checks.browser === "fail") {
    return {
      target: "browser",
      message:
        "The start button stays disabled until you open this page in a supported browser.",
    };
  }
  if (checks.mic === "fail") {
    return {
      target: "mic",
      message:
        "The start button stays disabled until the microphone check passes.",
    };
  }
  if (checks.mic !== "pass") {
    return {
      target: "mic",
      message: "Run the microphone check to enable the start button.",
    };
  }
  if (!requiredConsent) {
    return {
      target: "consent",
      message:
        "Agree to recording above to enable the start button. Nothing is recorded until you do.",
    };
  }
  return null;
}

/** What the browser must be able to do for an interview to happen at all. */
export interface BrowserCapabilities {
  hasGetUserMedia: boolean;
  hasAudioContext: boolean;
  hasWebSocket: boolean;
}

export function browserSupport(capabilities: BrowserCapabilities): boolean {
  return (
    capabilities.hasGetUserMedia &&
    capabilities.hasAudioContext &&
    capabilities.hasWebSocket
  );
}
