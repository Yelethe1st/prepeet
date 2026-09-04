/**
 * The browser's side of the control event protocol: RTC-03's resend buffer,
 * built on the acknowledgment and replay RTC-02 provides.
 *
 * The server's record is the record. What lives here is only what the server
 * has not yet confirmed: every durable event keeps its identity and its slot
 * until an acknowledgment covers it, a retry resends the whole unconfirmed
 * tail (duplicates converge server-side by event id), and a takeover rebases
 * the survivors into the new epoch with the same identities, so an event
 * that did land converges to a duplicate instead of doubling and one that
 * did not takes its place in the new numbering. The buffer never invents
 * order: events resend in the order they were recorded.
 */

/** One event on its way to the server, exactly the contract's shape. */
export interface WireEvent {
  event_id: string;
  sequence: number;
  type: string;
  payload?: Record<string, unknown>;
  occurred_at: string;
}

/** The acknowledgment's answer, narrowed to what the buffer acts on. */
export interface AckView {
  accepted_sequence: number;
  outcomes: {
    event_id: string;
    status: "accepted" | "duplicate" | "refused";
    reason?: string;
  }[];
}

export interface TimelineOptions {
  /** Posts one batch for the given epoch; throws on refusal or outage. */
  post: (epoch: number, events: WireEvent[]) => Promise<AckView>;
  /** The epoch the connection currently speaks. */
  epoch: number;
  /**
   * Called when the server refuses an event by name. The event is dropped,
   * because a refused event resent forever would jam the buffer behind it.
   */
  onRefused?: (event: WireEvent, reason: string) => void;
  /** Swappable for tests; default crypto.randomUUID and the wall clock. */
  makeId?: () => string;
  now?: () => Date;
}

export class Timeline {
  private readonly post: TimelineOptions["post"];
  private readonly onRefused: TimelineOptions["onRefused"];
  private readonly makeId: () => string;
  private readonly now: () => Date;

  private epoch: number;
  private nextSequence = 1;
  private unacked: WireEvent[] = [];
  private inFlight: Promise<void> | null = null;
  private flushAgain = false;

  constructor(options: TimelineOptions) {
    this.post = options.post;
    this.onRefused = options.onRefused;
    this.makeId = options.makeId ?? (() => crypto.randomUUID());
    this.now = options.now ?? (() => new Date());
    this.epoch = options.epoch;
  }

  /** The epoch the buffer currently numbers into. */
  get currentEpoch(): number {
    return this.epoch;
  }

  /** How many durable events await confirmation. */
  get pending(): number {
    return this.unacked.length;
  }

  /**
   * Records one durable event into the current epoch. Nothing is sent until
   * flush; recording is synchronous so an event can be captured on the way
   * out of a dying connection.
   */
  record(type: string, payload?: Record<string, unknown>): void {
    this.unacked.push({
      event_id: this.makeId(),
      sequence: this.nextSequence++,
      type,
      ...(payload === undefined ? {} : { payload }),
      occurred_at: this.now().toISOString(),
    });
  }

  /**
   * Sends everything unconfirmed and settles the buffer against the answer.
   * One batch in flight at a time: a flush during a flush runs after it, so
   * two callers cannot interleave the numbering. The returned promise
   * resolves when this caller's events have been offered once; a post that
   * throws leaves the buffer intact for the next flush or the recovery flow.
   */
  flush(): Promise<void> {
    if (this.inFlight) {
      this.flushAgain = true;
      return this.inFlight;
    }
    this.inFlight = this.deliver().finally(() => {
      this.inFlight = null;
      if (this.flushAgain) {
        this.flushAgain = false;
        if (this.unacked.length > 0) {
          void this.flush();
        }
      }
    });
    return this.inFlight;
  }

  private async deliver(): Promise<void> {
    if (this.unacked.length === 0) {
      return;
    }
    const batch = [...this.unacked];
    const ack = await this.post(this.epoch, batch);

    const settled = new Set<string>();
    for (const outcome of ack.outcomes) {
      if (outcome.status === "refused") {
        // Dropped, not retried: the server named why, and resending the
        // same refusal forever would jam everything queued behind it.
        settled.add(outcome.event_id);
        const refused = batch.find(
          (event) => event.event_id === outcome.event_id,
        );
        if (refused) {
          this.onRefused?.(refused, outcome.reason ?? "");
        }
        continue;
      }
      // Accepted and duplicate both mean the server holds it.
      settled.add(outcome.event_id);
    }
    this.unacked = this.unacked.filter(
      (event) =>
        !settled.has(event.event_id) && event.sequence > ack.accepted_sequence,
    );
  }

  /**
   * Rebases the unconfirmed tail into a new epoch after a resume.
   *
   * The recovery cursor says what the superseded epoch durably holds:
   * everything at or under it is confirmed and leaves the buffer, and the
   * survivors renumber from one in the order they were recorded, keeping
   * their identities so anything that did land beyond the contiguous cursor
   * converges to a duplicate rather than doubling.
   */
  rebase(newEpoch: number, previousAccepted: number): void {
    this.epoch = newEpoch;
    const survivors = this.unacked.filter(
      (event) => event.sequence > previousAccepted,
    );
    this.nextSequence = 1;
    this.unacked = survivors.map((event) => ({
      ...event,
      sequence: this.nextSequence++,
    }));
  }
}
