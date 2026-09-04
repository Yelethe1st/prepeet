/**
 * The reconnection chain: RTC-03, to realtime-protocol.md's recovery
 * contract.
 *
 * A dropped connection recovers into the same session or ends with a named
 * reason, never into a spinner. One attempt is the whole chain: resume for
 * a fresh epoch and grant, rejoin the room, rebase the resend buffer onto
 * the recovery cursor, then hand the server the interruption report and the
 * new epoch's established event. Any link failing retries the whole chain
 * on a backoff, because a half-recovered state - room joined, server never
 * told - would leave the grace timer running toward a finalization the
 * candidate cannot see; re-running the chain opens the next epoch and
 * converges instead.
 *
 * The server's refusals end the chain by name: GRACE_EXPIRED means the
 * window closed and what was captured is being finalized; EPOCH_STALE means
 * another connection owns the session now; SESSION_NOT_RESUMABLE means
 * there is no interview in flight to rejoin.
 */

/** What stopped the interview, in the closed vocabulary SES-06 records. */
export type DropCause = "connection_lost" | "device_failure";

/** The overlay's state: every phase names what is true and what comes next. */
export type RecoveryPhase =
  | { kind: "reconnecting"; attempt: number; maxAttempts: number }
  | { kind: "recovered" }
  | { kind: "expired" }
  | { kind: "unresumable" }
  | { kind: "superseded" }
  | { kind: "exhausted"; maxAttempts: number };

/** The resume answer, narrowed to what the chain consumes. */
export interface ResumeAnswer {
  grant: { url: string; token: string };
  epoch: number;
  previousAccepted: number;
}

export interface RecoveryDeps {
  /** POST /interviews/{id}/resume; throws with a coded refusal. */
  resume: () => Promise<ResumeAnswer>;
  /** Rejoins the room with the fresh grant; throws when it cannot. */
  reconnect: (grant: { url: string; token: string }) => Promise<void>;
  /** The resend buffer the chain reports through and rebases. */
  timeline: {
    record(type: string, payload?: Record<string, unknown>): void;
    flush(): Promise<void>;
    rebase(epoch: number, previousAccepted: number): void;
  };
  /** Receives every phase change; the overlay renders and announces it. */
  onPhase: (phase: RecoveryPhase) => void;
  /** Reads the stable code off a refusal; default reads `.code`. */
  refusalCode?: (error: unknown) => string;
  /** Waits before automatic attempts 2..n+1; the length caps the cycle. */
  delaysMs?: number[];
  /** Swappable clock and timer for tests. */
  schedule?: (run: () => void, ms: number) => () => void;
  now?: () => number;
}

const defaultDelays = [2_000, 5_000, 10_000, 20_000];

const defaultSchedule = (run: () => void, ms: number): (() => void) => {
  const timer = setTimeout(run, ms);
  return () => clearTimeout(timer);
};

const defaultRefusalCode = (error: unknown): string =>
  typeof (error as { code?: unknown })?.code === "string"
    ? (error as { code: string }).code
    : "";

export class Recovery {
  private readonly deps: RecoveryDeps;
  private readonly delays: number[];
  private readonly schedule: (run: () => void, ms: number) => () => void;
  private readonly now: () => number;
  private readonly refusalCode: (error: unknown) => string;

  private active = false;
  private droppedAt = 0;
  private cause: DropCause = "connection_lost";
  private cancelWait: (() => void) | null = null;

  constructor(deps: RecoveryDeps) {
    this.deps = deps;
    this.delays = deps.delaysMs ?? defaultDelays;
    this.schedule = deps.schedule ?? defaultSchedule;
    this.now = deps.now ?? (() => Date.now());
    this.refusalCode = deps.refusalCode ?? defaultRefusalCode;
  }

  /** True while a drop is being recovered from. */
  get recovering(): boolean {
    return this.active;
  }

  /**
   * Starts recovering from a drop. A second drop while one is being
   * recovered changes nothing: the chain already covers it.
   */
  begin(cause: DropCause): void {
    if (this.active) {
      return;
    }
    this.active = true;
    this.cause = cause;
    this.droppedAt = this.now();

    // Tell the server the connection is gone, best effort in the dying
    // epoch: this is what folds the session to reconnecting and starts the
    // grace timer honestly. A tab that cannot reach the server loses only
    // the report; resume itself is the recovery.
    this.deps.timeline.record("connection.lost");
    void this.deps.timeline.flush().catch(() => undefined);

    void this.attempt(1);
  }

  /**
   * Retries immediately on the person's own say-so, from the waiting or the
   * exhausted state alike, restarting the automatic cycle.
   */
  retryNow(): void {
    if (!this.active) {
      return;
    }
    this.cancelWait?.();
    this.cancelWait = null;
    void this.attempt(1);
  }

  /** Stops recovering without a verdict: the surface is going away. */
  cancel(): void {
    this.cancelWait?.();
    this.cancelWait = null;
    this.active = false;
  }

  private async attempt(n: number): Promise<void> {
    this.deps.onPhase({
      kind: "reconnecting",
      attempt: n,
      maxAttempts: this.delays.length + 1,
    });

    let answer: ResumeAnswer;
    try {
      answer = await this.deps.resume();
    } catch (error) {
      switch (this.refusalCode(error)) {
        case "GRACE_EXPIRED":
          this.conclude({ kind: "expired" });
          return;
        case "SESSION_NOT_RESUMABLE":
          this.conclude({ kind: "unresumable" });
          return;
        case "EPOCH_STALE":
          this.conclude({ kind: "superseded" });
          return;
        default:
          this.next(n);
          return;
      }
    }

    try {
      await this.deps.reconnect(answer.grant);
      this.deps.timeline.rebase(answer.epoch, answer.previousAccepted);
      // The report the humans deciding on re-invitation need: what stopped
      // the interview and for how long, from the one layer that saw it.
      this.deps.timeline.record("interruption", {
        cause: this.cause,
        duration_seconds: Math.max(
          0,
          Math.round((this.now() - this.droppedAt) / 1000),
        ),
      });
      // The new epoch's established event is what returns the session to
      // in_progress; recovery is not claimed until the server has it.
      this.deps.timeline.record("connection.established");
      await this.deps.timeline.flush();
    } catch {
      this.next(n);
      return;
    }

    this.conclude({ kind: "recovered" });
  }

  private next(n: number): void {
    if (!this.active) {
      return;
    }
    if (n > this.delays.length) {
      // Out of automatic attempts. Still recovering: the person keeps the
      // retry button, and the grace window is the server's to enforce.
      this.deps.onPhase({
        kind: "exhausted",
        maxAttempts: this.delays.length + 1,
      });
      return;
    }
    this.cancelWait = this.schedule(
      () => {
        this.cancelWait = null;
        void this.attempt(n + 1);
      },
      this.delays[n - 1] ?? 0,
    );
  }

  private conclude(phase: RecoveryPhase): void {
    if (!this.active) {
      return;
    }
    this.active = false;
    this.deps.onPhase(phase);
  }
}
