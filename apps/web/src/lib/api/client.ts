import type { components } from "@contracts";

/**
 * The browser's client for the Prepeet API.
 *
 * Thin on purpose. Everything specific to an endpoint lives beside the feature
 * that calls it; what is here is the handling every call shares, so that
 * credentials, the error envelope and the difference between an outage and a
 * refusal are decided once rather than per call site.
 *
 * The request and response types come from the generated contract, so a change
 * to the document that the server must honour also fails to compile here.
 */

/** The error envelope, exactly as the contract defines it. */
type ErrorEnvelope = components["schemas"]["Error"];

/**
 * basePath is the version prefix from the contract's servers block.
 *
 * ADR-0004 puts the version in the path because a new version is a project
 * rather than a release. Written once here so the client and the server cannot
 * disagree about where the API is.
 */
const basePath = "/api/v1";

/**
 * baseUrl lets the browser talk to an API on another origin, which is the
 * arrangement in every deployed environment.
 *
 * Empty by default, meaning same-origin, which is what the local stack is. A
 * cross-origin value makes `credentials: "include"` load bearing rather than
 * merely correct.
 */
const baseUrl = process.env.NEXT_PUBLIC_API_URL ?? "";

/** A message for a person, used when the server did not provide one. */
const fallbackMessage = "Something went wrong. Please try again.";
const offlineMessage =
  "We could not reach Prepeet. Check your connection and try again.";

/**
 * ApiError is every way a call can fail, in one shape.
 *
 * One type rather than a union, because a caller almost always wants the same
 * three things: whether to show a field error, what to say, and what identifier
 * to quote. A union would push that decision to every call site.
 */
export class ApiError extends Error {
  /** The HTTP status, or 0 when the request never reached a server. */
  readonly status: number;
  /** The stable code from the envelope, or an empty string when there was none. */
  readonly code: string;
  /** Whether the caller may retry the same request unchanged. */
  readonly retryable: boolean;
  /** Field messages keyed by field name, for showing beside the input. */
  readonly fieldErrors: Readonly<Record<string, string>>;
  /** The correlation identifier a person can quote to support. */
  readonly requestId: string;
  /**
   * Seconds until a retry could succeed, from the Retry-After header, or 0.
   * Carried so a cooldown can be shown as a countdown rather than a refusal;
   * the resend button on check-email is the reason it exists.
   */
  readonly retryAfterSeconds: number;
  /** True when the request never reached a server, which reads differently. */
  readonly offline: boolean;

  constructor(init: {
    status: number;
    code?: string;
    message: string;
    retryable?: boolean;
    fieldErrors?: Record<string, string>;
    requestId?: string;
    offline?: boolean;
    retryAfterSeconds?: number;
  }) {
    super(init.message);
    this.name = "ApiError";
    this.status = init.status;
    this.code = init.code ?? "";
    this.retryable = init.retryable ?? false;
    this.fieldErrors = init.fieldErrors ?? {};
    this.requestId = init.requestId ?? "";
    this.offline = init.offline ?? false;
    this.retryAfterSeconds = init.retryAfterSeconds ?? 0;
  }
}

/** What a caller may pass. `body` is serialised; everything else is fetch's. */
export interface ApiRequest extends Omit<RequestInit, "body"> {
  body?: unknown;
}

/**
 * apiFetch performs one call and returns the decoded body.
 *
 * It throws ApiError for anything that is not a success, so a caller writes the
 * happy path and handles failure in one place rather than branching on status.
 */
export async function apiFetch<T = unknown>(
  path: string,
  request: ApiRequest = {},
): Promise<T> {
  const { body, headers, ...rest } = request;

  const init: RequestInit = {
    ...rest,
    // Session tokens live in HttpOnly cookies so that no script can read them,
    // which also means the only way they reach the server is for this to be
    // set. fetch omits credentials cross-origin by default, and the deployed
    // arrangement is cross-origin, so this is load bearing rather than tidy.
    credentials: "include",
    headers: new Headers(headers),
  };

  if (body !== undefined) {
    (init.headers as Headers).set("content-type", "application/json");
    init.body = JSON.stringify(body);
  }

  let response: Response;
  try {
    response = await fetch(`${baseUrl}${basePath}${path}`, init);
  } catch {
    // fetch rejects only when the request never completed: offline, DNS, a
    // blocked request. A person seeing "check your connection" because they
    // mistyped a password is worse than unhelpful, so this is kept distinct
    // from anything the server said.
    throw new ApiError({
      status: 0,
      message: offlineMessage,
      offline: true,
      retryable: true,
    });
  }

  // No special case for 204. readJson already returns undefined for an empty
  // body, so a branch here changes nothing, and a branch that changes nothing
  // is a line no test can hold to account. Removing it was prompted by exactly
  // that: deleting it left the suite green.
  const payload = await readJson(response);

  if (!response.ok) {
    throw toApiError(response, payload);
  }

  return payload as T;
}

/**
 * readJson decodes a body, returning undefined rather than throwing.
 *
 * A failure that is not the envelope is the case worth surviving: a proxy error
 * page, a gateway timeout, an outage returning HTML. Those arrive exactly when
 * the interface most needs to show something rather than crash on a parse.
 */
async function readJson(response: Response): Promise<unknown> {
  const text = await response.text().catch(() => "");
  if (text === "") return undefined;

  try {
    return JSON.parse(text) as unknown;
  } catch {
    return undefined;
  }
}

/** toApiError builds the failure from whatever the server actually sent. */
function toApiError(response: Response, payload: unknown): ApiError {
  const envelope = (payload as ErrorEnvelope | undefined)?.error;

  if (!envelope) {
    // Deliberately not the body. An unparsed body here is a proxy's HTML or an
    // upstream's error text, and showing either to a person tells them nothing
    // and may tell them about the deployment.
    return new ApiError({
      status: response.status,
      message: fallbackMessage,
      retryable: true,
      retryAfterSeconds: retryAfterOf(response),
    });
  }

  const fieldErrors: Record<string, string> = {};
  for (const field of envelope.field_errors ?? []) {
    fieldErrors[field.field] = field.message;
  }

  return new ApiError({
    status: response.status,
    code: envelope.code,
    message: envelope.message || fallbackMessage,
    retryable: envelope.retryable,
    fieldErrors,
    requestId: envelope.request_id,
    retryAfterSeconds: retryAfterOf(response),
  });
}

/**
 * retryAfterOf reads the Retry-After header as whole seconds.
 *
 * Only the delta-seconds form, because that is what this API sends; the
 * HTTP-date form comes back as 0 rather than a NaN countdown.
 */
function retryAfterOf(response: Response): number {
  const parsed = Number.parseInt(response.headers.get("Retry-After") ?? "", 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
}
