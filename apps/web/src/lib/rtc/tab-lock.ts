/**
 * Duplicate tab detection: RTC-03's second box.
 *
 * One session, one live tab. The tab that holds the interview answers for
 * it on a per-session broadcast channel; a tab that wants it asks first,
 * and an answer within the window means somebody already has it, so the
 * newcomer refuses to go live instead of superseding an interview that is
 * mid-sentence. Detection is advisory - two tabs claiming in the same
 * instant can both hear silence - and the server's epoch takeover remains
 * the authority: whichever tab actually resumes owns the session, and the
 * other is refused by name with EPOCH_STALE.
 */

/** The structural surface the lock needs; BroadcastChannel has it. */
export interface ChannelLike {
  postMessage(message: unknown): void;
  close(): void;
  onmessage: ((event: { data: unknown }) => void) | null;
}

export interface ClaimOptions {
  /** Swappable for tests; defaults to BroadcastChannel. */
  createChannel?: (name: string) => ChannelLike;
  /** How long silence must last before the claim is granted. */
  timeoutMs?: number;
  /** Swappable for tests; defaults to setTimeout. */
  schedule?: (run: () => void, ms: number) => void;
}

export interface Claim {
  /** False when another tab already holds this session live. */
  granted: boolean;
  /** Stops answering for the session and closes the channel. Idempotent. */
  release: () => void;
}

/**
 * Asks whether any other tab holds the session live, and on silence takes
 * the role of answering for it. A browser without BroadcastChannel grants
 * every claim: no detection is possible there, and refusing everyone would
 * lock a person out of their own interview.
 */
export function claimLiveTab(
  sessionId: string,
  options: ClaimOptions = {},
): Promise<Claim> {
  const create =
    options.createChannel ??
    (typeof BroadcastChannel === "undefined"
      ? null
      : (name: string) => new BroadcastChannel(name) as ChannelLike);
  if (!create) {
    return Promise.resolve({ granted: true, release: () => undefined });
  }

  const channel = create(`prepeet-live-${sessionId}`);
  const schedule = options.schedule ?? ((run, ms) => void setTimeout(run, ms));
  const timeoutMs = options.timeoutMs ?? 250;

  return new Promise<Claim>((resolve) => {
    let settled = false;
    let released = false;

    const release = (): void => {
      if (released) {
        return;
      }
      released = true;
      channel.onmessage = null;
      channel.close();
    };

    channel.onmessage = (event): void => {
      const data = event.data as { kind?: string } | null;
      if (!settled && data?.kind === "occupied") {
        settled = true;
        release();
        resolve({ granted: false, release: () => undefined });
      }
    };

    channel.postMessage({ kind: "claim" });

    schedule(() => {
      if (settled) {
        return;
      }
      settled = true;
      // Silence: the session is ours to hold. From here on, answer anyone
      // who asks, so the next duplicate refuses instead of joining.
      channel.onmessage = (event): void => {
        const data = event.data as { kind?: string } | null;
        if (data?.kind === "claim") {
          channel.postMessage({ kind: "occupied" });
        }
      };
      resolve({ granted: true, release });
    }, timeoutMs);
  });
}
