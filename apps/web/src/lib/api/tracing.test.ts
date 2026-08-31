import { afterEach, describe, expect, it } from "vitest";

import {
  resetTracingForTests,
  startClientTrace,
  traceHeaders,
} from "./tracing";

/**
 * PLT-08's remaining break is the first hop. The trace begins at the Go edge
 * rather than at the click, so the slowest thing a candidate experiences, the
 * browser waiting, is outside every trace of the request it made.
 *
 * The rule these encode is the one the Go and Python sides already follow, and
 * it cuts the other way here. Absent context is honest and invented context is
 * not, so a browser that is not recording sends no traceparent at all rather
 * than a well-formed one naming a span nobody exported. That would be the
 * dangling parent telemetry-conventions.md forbids, arriving from the one place
 * with the best excuse for it.
 */
describe("browser trace propagation", () => {
  afterEach(() => {
    resetTracingForTests();
  });

  it("sends no traceparent when tracing is not enabled", () => {
    resetTracingForTests({ enabled: false });

    expect(traceHeaders()).toEqual({});
  });

  it("sends a traceparent once tracing is enabled", () => {
    resetTracingForTests({ enabled: true });
    startClientTrace();

    const headers = traceHeaders();

    expect(headers.traceparent).toMatch(/^00-[0-9a-f]{32}-[0-9a-f]{16}-0[01]$/);
  });

  it("keeps one trace across the calls of a single interaction", () => {
    resetTracingForTests({ enabled: true });
    startClientTrace();

    const first = traceHeaders().traceparent;
    const second = traceHeaders().traceparent;

    // A screen that loads a profile and a session list made one gesture, and
    // splitting it into two traces means nobody can see them as one wait.
    expect(traceId(first)).toBe(traceId(second));
  });

  it("gives each call its own span within that trace", () => {
    resetTracingForTests({ enabled: true });
    startClientTrace();

    const first = traceHeaders().traceparent;
    const second = traceHeaders().traceparent;

    // Same trace, different spans: two calls that shared a span id would make
    // the server's children collide onto one parent that never existed twice.
    expect(spanId(first)).not.toBe(spanId(second));
  });

  it("starts a new trace for a new interaction", () => {
    resetTracingForTests({ enabled: true });
    startClientTrace();
    const before = traceId(traceHeaders().traceparent);

    startClientTrace();
    const after = traceId(traceHeaders().traceparent);

    expect(after).not.toBe(before);
  });

  it("sends nothing when enabled but no interaction has started", () => {
    // Enabling the exporter is not the same as being inside a recorded
    // interaction. Emitting here would name a root that does not exist.
    resetTracingForTests({ enabled: true });

    expect(traceHeaders()).toEqual({});
  });

  it("marks the trace as sampled so the server records it too", () => {
    resetTracingForTests({ enabled: true });
    startClientTrace();

    // The flags byte is what tells every downstream hop to record. A trace
    // that begins unsampled is a trace that exists only as an id.
    expect(traceHeaders().traceparent?.endsWith("-01")).toBe(true);
  });

  it("never reuses a trace id across interactions", () => {
    resetTracingForTests({ enabled: true });

    const seen = new Set<string>();
    for (let i = 0; i < 50; i += 1) {
      startClientTrace();
      seen.add(traceId(traceHeaders().traceparent));
    }

    expect(seen.size).toBe(50);
  });
});

function traceId(traceparent: string | undefined): string {
  return traceparent?.split("-")[1] ?? "";
}

function spanId(traceparent: string | undefined): string {
  return traceparent?.split("-")[2] ?? "";
}
