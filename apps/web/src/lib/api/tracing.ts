/**
 * W3C trace context, generated where the journey actually starts.
 *
 * PLT-08 asks for one trace across browser, Go, workflow, Python, provider,
 * database and object storage. The server-side hops are joined; this is the
 * first one. Without it a trace begins at the Go edge, which leaves the slowest
 * thing a candidate experiences, the browser waiting, outside every trace of
 * the request that caused it.
 *
 * The format is the same W3C trace context the outbox stores and the gRPC
 * client sends, for the same reason: no part of this journey needs a private
 * agreement with another part in order to be joined to it.
 *
 * ## Why this sends nothing by default
 *
 * telemetry-conventions.md says a span attached to a parent that cannot exist
 * looks joined and leads nowhere, and is worse than no parent at all. A browser
 * that emits a traceparent while exporting no spans creates exactly that: every
 * server trace would name a root nobody can fetch. So propagation is tied to
 * recording rather than switched on independently, and a browser that is not
 * recording sends no header. Absent context is honest; invented context is not.
 *
 * Client-side span export is not built here. That needs a collector reachable
 * from the browser, which is a deployment decision rather than a code one, and
 * the ticket note says so rather than implying this hop is finished.
 */

/** Config for the browser's half of the trace. */
export interface TracingConfig {
  /**
   * Whether this browser records and therefore propagates.
   *
   * Off unless a deployment turns it on, because propagating without recording
   * would produce traces whose root is permanently missing.
   */
  enabled: boolean;
}

/** The identifiers of one browser interaction. */
interface ClientTrace {
  traceID: string;
  /** Incremented per call so each request is its own span, not a shared one. */
  calls: number;
}

let config: TracingConfig = { enabled: false };
let current: ClientTrace | undefined;

/**
 * configureTracing installs the deployment's choice.
 *
 * Read from the environment by the caller rather than here, so this module can
 * be tested without reaching for process.env and so a server-rendered pass
 * cannot accidentally pick up a browser setting.
 */
export function configureTracing(next: TracingConfig): void {
  config = next;
}

/**
 * startClientTrace begins a new trace for one interaction.
 *
 * An interaction rather than a request: a screen that loads a profile and a
 * session list made one gesture, and splitting that into two traces means
 * nobody can see it as the single wait the person actually experienced.
 */
export function startClientTrace(): void {
  if (!config.enabled) {
    current = undefined;
    return;
  }
  current = { traceID: randomHex(32), calls: 0 };
}

/**
 * traceHeaders returns the headers to add to one outbound call.
 *
 * Empty when there is nothing honest to send: tracing off, or on but outside
 * any started interaction. Both are real states, and neither is an error.
 */
export function traceHeaders(): Record<string, string> {
  if (!config.enabled || current === undefined) {
    return {};
  }
  current.calls += 1;
  // Sampled. A trace that begins unsampled is an id with nothing behind it,
  // and every downstream hop would honour that decision.
  return {
    traceparent: `00-${current.traceID}-${randomHex(16)}-01`,
  };
}

/**
 * randomHex produces a lowercase hex string of the given length.
 *
 * crypto.getRandomValues rather than Math.random. Trace ids are not secrets,
 * but they must not collide: two interactions sharing an id merge into one
 * trace that describes neither, and Math.random collides far sooner than its
 * range suggests.
 */
function randomHex(length: number): string {
  const bytes = new Uint8Array(length / 2);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join(
    "",
  );
}

/**
 * resetTracingForTests clears module state between tests.
 *
 * Exported because the state is deliberately module level: one interaction is a
 * property of the page, not of a component, and threading it through every
 * caller would put the decision in the hands of whoever writes the next screen.
 */
export function resetTracingForTests(next?: TracingConfig): void {
  config = next ?? { enabled: false };
  current = undefined;
}
