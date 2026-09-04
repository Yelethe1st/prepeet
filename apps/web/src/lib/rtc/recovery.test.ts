import { describe, expect, it } from "vitest";

import {
  Recovery,
  type RecoveryDeps,
  type RecoveryPhase,
  type ResumeAnswer,
} from "./recovery";

/**
 * RTC-03's recovery chain. Each interruption class the ticket names is a
 * scenario here at the controller's level: sleep and wake recover on the
 * first chain, a network handover on a later one, a removed device once it
 * is back, and the server's refusals end the chain by name. What the tests
 * hold constant: the server is told about the drop, the buffer rebases onto
 * the recovery cursor, and recovery is never claimed before the new epoch's
 * established event was delivered.
 */

const answer: ResumeAnswer = {
  grant: { url: "wss://rtc.test", token: "tok-2" },
  epoch: 2,
  previousAccepted: 3,
};

function coded(code: string): Error & { code: string } {
  return Object.assign(new Error(code), { code });
}

function harness(overrides: Partial<RecoveryDeps> = {}) {
  const phases: RecoveryPhase[] = [];
  const recorded: { type: string; payload?: Record<string, unknown> }[] = [];
  const rebases: [number, number][] = [];
  const flushes: { fail: boolean }[] = [];
  const timers: {
    run: () => void;
    ms: number;
    cancelled: boolean;
    fired: boolean;
  }[] = [];
  let flushFailures = 0;
  let clock = 100_000;

  const deps: RecoveryDeps = {
    resume: () => Promise.resolve(answer),
    reconnect: () => Promise.resolve(),
    timeline: {
      record: (type, payload) => {
        recorded.push(payload === undefined ? { type } : { type, payload });
      },
      flush: () => {
        const fail = flushFailures > 0;
        if (fail) {
          flushFailures -= 1;
        }
        flushes.push({ fail });
        return fail ? Promise.reject(new Error("offline")) : Promise.resolve();
      },
      rebase: (epoch, previousAccepted) => {
        rebases.push([epoch, previousAccepted]);
      },
    },
    onPhase: (phase) => phases.push(phase),
    delaysMs: [1_000, 2_000],
    schedule: (run, ms) => {
      const timer = { run, ms, cancelled: false, fired: false };
      timers.push(timer);
      return () => {
        timer.cancelled = true;
      };
    },
    now: () => clock,
    ...overrides,
  };

  return {
    recovery: new Recovery(deps),
    phases,
    recorded,
    rebases,
    flushes,
    timers,
    failNextFlushes: (n: number) => {
      flushFailures = n;
    },
    tick: async () => {
      // Let the chain's promises settle between assertions.
      await new Promise((resolve) => setTimeout(resolve, 0));
    },
    advance: async (count = 1) => {
      for (let index = 0; index < count; index += 1) {
        const timer = timers.find((entry) => !entry.cancelled && !entry.fired);
        if (timer) {
          timer.fired = true;
          timer.run();
        }
        await new Promise((resolve) => setTimeout(resolve, 0));
      }
    },
    passTime: (ms: number) => {
      clock += ms;
    },
  };
}

describe("recovering into the same session", () => {
  it("sleep and wake: one chain, told to the server end to end", async () => {
    const h = harness();

    h.recovery.begin("connection_lost");
    h.passTime(42_000);
    await h.tick();

    // The dying epoch was told first, best effort.
    expect(h.recorded[0]).toEqual({ type: "connection.lost" });

    // The buffer rebased onto exactly the cursor resume answered.
    expect(h.rebases).toEqual([[2, 3]]);

    // The interruption report carries the cause and the measured gap, and
    // recovery is claimed only after the established event flushed.
    expect(h.recorded[1]).toEqual({
      type: "interruption",
      payload: { cause: "connection_lost", duration_seconds: 42 },
    });
    expect(h.recorded[2]).toEqual({ type: "connection.established" });
    expect(h.phases).toEqual([
      { kind: "reconnecting", attempt: 1, maxAttempts: 3 },
      { kind: "recovered" },
    ]);
    expect(h.recovery.recovering).toBe(false);
  });

  it("network handover: a failed resume retries on the backoff and recovers", async () => {
    let calls = 0;
    const h = harness({
      resume: () => {
        calls += 1;
        return calls === 1
          ? Promise.reject(new Error("offline"))
          : Promise.resolve(answer);
      },
    });

    h.recovery.begin("connection_lost");
    await h.tick();
    expect(h.phases).toEqual([
      { kind: "reconnecting", attempt: 1, maxAttempts: 3 },
    ]);
    expect(h.timers[0]?.ms).toBe(1_000);

    await h.advance();
    expect(h.phases).toEqual([
      { kind: "reconnecting", attempt: 1, maxAttempts: 3 },
      { kind: "reconnecting", attempt: 2, maxAttempts: 3 },
      { kind: "recovered" },
    ]);
  });

  it("device removal: a failed rejoin retries and carries device_failure in the report", async () => {
    let joins = 0;
    const h = harness({
      reconnect: () => {
        joins += 1;
        return joins === 1
          ? Promise.reject(new Error("microphone"))
          : Promise.resolve();
      },
    });

    h.recovery.begin("device_failure");
    await h.tick();
    await h.advance();

    expect(h.phases.at(-1)).toEqual({ kind: "recovered" });
    const report = h.recorded.find((event) => event.type === "interruption");
    expect(report?.payload?.cause).toBe("device_failure");
  });

  it("a handshake that cannot be delivered retries the whole chain rather than half-recovering", async () => {
    const h = harness();
    // The drop report and the first chain's handshake both fail; the second
    // chain's handshake lands.
    h.failNextFlushes(2);

    h.recovery.begin("connection_lost");
    await h.tick();
    expect(h.phases.at(-1)).toEqual({
      kind: "reconnecting",
      attempt: 1,
      maxAttempts: 3,
    });

    await h.advance();
    expect(h.phases.at(-1)).toEqual({ kind: "recovered" });
    // Two chains, two rebases: the second resume owns the session now.
    expect(h.rebases).toHaveLength(2);
  });
});

describe("the server's refusals end the chain by name", () => {
  const refusals: [string, RecoveryPhase["kind"]][] = [
    ["GRACE_EXPIRED", "expired"],
    ["SESSION_NOT_RESUMABLE", "unresumable"],
    ["EPOCH_STALE", "superseded"],
  ];

  for (const [code, kind] of refusals) {
    it(`${code} concludes as ${kind} with no further attempts`, async () => {
      const h = harness({ resume: () => Promise.reject(coded(code)) });

      h.recovery.begin("connection_lost");
      await h.tick();

      expect(h.phases.at(-1)?.kind).toBe(kind);
      expect(h.recovery.recovering).toBe(false);
      expect(h.timers).toHaveLength(0);
    });
  }
});

describe("attempts, exhaustion and the person's own retry", () => {
  it("runs out of automatic attempts and waits on the person", async () => {
    const h = harness({ resume: () => Promise.reject(new Error("offline")) });

    h.recovery.begin("connection_lost");
    await h.tick();
    await h.advance(2);

    expect(h.phases.at(-1)).toEqual({ kind: "exhausted", maxAttempts: 3 });
    // Still recovering: the person keeps the retry button.
    expect(h.recovery.recovering).toBe(true);
  });

  it("retry now restarts the cycle and can still recover", async () => {
    let calls = 0;
    const h = harness({
      resume: () => {
        calls += 1;
        return calls < 4
          ? Promise.reject(new Error("offline"))
          : Promise.resolve(answer);
      },
    });

    h.recovery.begin("connection_lost");
    await h.tick();
    await h.advance(2);
    expect(h.phases.at(-1)?.kind).toBe("exhausted");

    h.recovery.retryNow();
    await h.tick();

    expect(h.phases.at(-1)).toEqual({ kind: "recovered" });
  });

  it("retry now while waiting cancels the pending wait instead of doubling it", async () => {
    let calls = 0;
    const h = harness({
      resume: () => {
        calls += 1;
        return calls === 1
          ? Promise.reject(new Error("offline"))
          : Promise.resolve(answer);
      },
    });

    h.recovery.begin("connection_lost");
    await h.tick();
    h.recovery.retryNow();
    await h.tick();

    expect(h.timers[0]?.cancelled).toBe(true);
    expect(h.phases.at(-1)).toEqual({ kind: "recovered" });
  });

  it("a second drop during recovery changes nothing", async () => {
    const h = harness({ resume: () => Promise.reject(new Error("offline")) });

    h.recovery.begin("connection_lost");
    await h.tick();
    h.recovery.begin("connection_lost");
    await h.tick();

    // One chain: one drop report, one first attempt.
    expect(
      h.recorded.filter((event) => event.type === "connection.lost"),
    ).toHaveLength(1);
    expect(
      h.phases.filter((phase) => phase.kind === "reconnecting"),
    ).toHaveLength(1);
  });

  it("cancel stops the chain without a verdict", async () => {
    const h = harness({ resume: () => Promise.reject(new Error("offline")) });

    h.recovery.begin("connection_lost");
    await h.tick();
    h.recovery.cancel();
    await h.advance();

    expect(h.phases).toEqual([
      { kind: "reconnecting", attempt: 1, maxAttempts: 3 },
    ]);
    expect(h.recovery.recovering).toBe(false);
  });

  it("retry now and cancel outside a recovery do nothing", () => {
    const h = harness();

    h.recovery.retryNow();
    h.recovery.cancel();

    expect(h.phases).toEqual([]);
  });
});
