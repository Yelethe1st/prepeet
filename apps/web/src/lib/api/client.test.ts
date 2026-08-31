import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError, apiFetch } from "./client";
import { resetTracingForTests, startClientTrace } from "./tracing";

/**
 * The browser's side of the contract.
 *
 * What is asserted here is the handling every call shares: credentials, the
 * error envelope, and what happens when the response is not what the contract
 * describes. Whether a particular endpoint returns the right thing is settled
 * on the server, against a real database, and repeating it here would be two
 * places to update and one that gets forgotten.
 */

/**
 * The paths the browser may name are the paths the contract declares.
 *
 * Checked by the compiler rather than at run time, because a path that does not
 * exist fails as a 404 on somebody's screen, and the whole point of generating
 * the client from the document is that the browser cannot ask for a route the
 * server never agreed to serve. CTR-01 requires the contract to cover every
 * route this phase ships; this is the half that proves the browser ships no
 * route the contract is missing.
 *
 * `@ts-expect-error` is the assertion: if apiFetch ever went back to accepting
 * any string, the directive would have nothing to suppress and `pnpm typecheck`
 * would fail on the unused directive. The test that follows keeps this block
 * from being deleted as dead code.
 */
function pathsAreCheckedAgainstTheContract() {
  // Real operations, named exactly as the document names them.
  void apiFetch("/me/profile");
  void apiFetch("/catalog/disciplines");
  // A parameterised operation, with the parameter substituted.
  void apiFetch(`/interviews/${"a-session"}/start`, { method: "POST" });
  // @ts-expect-error the contract declares no /me/nonexistent
  void apiFetch("/me/nonexistent");
  // @ts-expect-error a fixed segment after a parameter still has to match
  void apiFetch(`/me/facts/${"a-fact"}/reviewed`, { method: "POST" });
  // @ts-expect-error public-api.md lists campaigns; the contract does not
  // declare them yet, so the browser cannot call them by accident
  void apiFetch("/tenant/campaigns");
}

describe("the paths a caller may name", () => {
  it("is settled by the compiler, not by this test", () => {
    // The assertion really is the three directives above: `pnpm typecheck`
    // fails on an unused `@ts-expect-error`, so widening apiFetch back to any
    // string breaks the build. This names the block so that it is not removed
    // as unused code, and so that a reader looking for the check finds it.
    expect(pathsAreCheckedAgainstTheContract).toBeInstanceOf(Function);
  });
});

const fetchMock = vi.fn();

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
});

function jsonResponse(
  status: number,
  body: unknown,
  headers: Record<string, string> = {},
) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json", ...headers },
  });
}

describe("apiFetch", () => {
  it("sends cookies, or the session never leaves the browser", async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { user_id: "u" }));

    await apiFetch("/me");

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    // Session tokens live in HttpOnly cookies precisely so no script can read
    // them, which means the only way they reach the server is for fetch to be
    // told to include them. The default is to omit them for cross-origin
    // requests, so this is not something to leave to a default.
    expect(init.credentials).toBe("include");
  });

  it("addresses the versioned path from the contract", async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, {}));

    await apiFetch("/me");

    const [url] = fetchMock.mock.calls[0] as [string];
    expect(url).toContain("/api/v1/me");
  });

  it("sends a JSON body and says so", async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, {}));

    await apiFetch("/auth/login", {
      method: "POST",
      body: { email: "a@b.co", password: "x" },
    });

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(init.method).toBe("POST");
    expect(new Headers(init.headers).get("content-type")).toContain(
      "application/json",
    );
    expect(JSON.parse(init.body as string)).toEqual({
      email: "a@b.co",
      password: "x",
    });
  });

  it("returns the decoded body on success", async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { user_id: "usr_1" }));

    await expect(apiFetch("/me")).resolves.toEqual({ user_id: "usr_1" });
  });

  it("returns nothing for a 204, rather than failing to parse an empty body", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));

    await expect(
      apiFetch("/auth/logout", { method: "POST" }),
    ).resolves.toBeUndefined();
  });

  // ─────────────────────────────────────────────────── the error envelope

  it("raises an ApiError carrying the envelope", async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(401, {
        error: {
          code: "UNAUTHENTICATED",
          message: "Those details did not sign you in.",
          retryable: false,
          field_errors: [],
          request_id: "req_01a03",
        },
      }),
    );

    const failure = await apiFetch("/auth/login", { method: "POST" }).catch(
      (e: unknown) => e,
    );

    expect(failure).toBeInstanceOf(ApiError);
    const error = failure as ApiError;
    expect(error.status).toBe(401);
    expect(error.code).toBe("UNAUTHENTICATED");
    expect(error.message).toBe("Those details did not sign you in.");
    // The identifier is what a person quotes to support, so it must survive to
    // somewhere the interface can show it.
    expect(error.requestId).toBe("req_01a03");
  });

  it("exposes field errors keyed by field, which is how a form shows them", async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(400, {
        error: {
          code: "VALIDATION_FAILED",
          message: "Some of the details were not accepted.",
          retryable: false,
          field_errors: [
            {
              field: "password",
              code: "PASSWORD_INVALID",
              message: "Too short.",
            },
            {
              field: "email",
              code: "EMAIL_INVALID",
              message: "Not deliverable.",
            },
          ],
          request_id: "req_01a03",
        },
      }),
    );

    const error = (await apiFetch("/auth/register", { method: "POST" }).catch(
      (e: unknown) => e,
    )) as ApiError;

    expect(error.fieldErrors).toEqual({
      password: "Too short.",
      email: "Not deliverable.",
    });
  });

  /**
   * A failure that is not the envelope is the interesting one: a proxy error
   * page, a gateway timeout, an outage returning HTML. The interface must show
   * something rather than crash on a parse, and it must not show the HTML.
   */
  it("survives a failure that is not the error envelope", async () => {
    fetchMock.mockResolvedValue(
      new Response("<html><body>502 Bad Gateway</body></html>", {
        status: 502,
        headers: { "content-type": "text/html" },
      }),
    );

    const error = (await apiFetch("/me").catch((e: unknown) => e)) as ApiError;

    expect(error).toBeInstanceOf(ApiError);
    expect(error.status).toBe(502);
    expect(error.message).not.toContain("<html>");
    expect(error.message.length).toBeGreaterThan(0);
  });

  it("survives a body that claims to be JSON and is not", async () => {
    fetchMock.mockResolvedValue(
      new Response("{not json", {
        status: 500,
        headers: { "content-type": "application/json" },
      }),
    );

    const error = (await apiFetch("/me").catch((e: unknown) => e)) as ApiError;

    expect(error).toBeInstanceOf(ApiError);
    expect(error.status).toBe(500);
  });

  /**
   * The network failing is not the same as the server refusing, and a person
   * seeing "check your connection" when they mistyped a password is worse than
   * unhelpful.
   */
  it("distinguishes the network failing from the server answering", async () => {
    fetchMock.mockRejectedValue(new TypeError("Failed to fetch"));

    const error = (await apiFetch("/me").catch((e: unknown) => e)) as ApiError;

    expect(error).toBeInstanceOf(ApiError);
    expect(error.status).toBe(0);
    expect(error.offline).toBe(true);
  });

  it("reports a server answer as not offline", async () => {
    fetchMock.mockResolvedValue(jsonResponse(500, {}));

    const error = (await apiFetch("/me").catch((e: unknown) => e)) as ApiError;

    expect(error.offline).toBe(false);
  });
});

describe("trace propagation on the wire", () => {
  afterEach(() => {
    resetTracingForTests();
  });

  it("sends no traceparent when the browser is not recording", async () => {
    resetTracingForTests({ enabled: false });
    fetchMock.mockResolvedValue(jsonResponse(200, {}));

    await apiFetch("/me");

    const headers = new Headers(fetchMock.mock.calls[0]?.[1]?.headers);
    expect(headers.has("traceparent")).toBe(false);
  });

  it("sends the traceparent on every call once recording", async () => {
    // Added at the one place every request passes through, so this is really
    // asserting that no call site has to remember. A trace with holes in it is
    // the failure, and it looks exactly like a working one until you need it.
    resetTracingForTests({ enabled: true });
    startClientTrace();
    fetchMock.mockResolvedValue(jsonResponse(200, {}));

    await apiFetch("/me");
    await apiFetch("/me/sessions");

    const first = new Headers(fetchMock.mock.calls[0]?.[1]?.headers);
    const second = new Headers(fetchMock.mock.calls[1]?.[1]?.headers);
    expect(first.get("traceparent")).toBeTruthy();
    expect(second.get("traceparent")).toBeTruthy();
    expect(first.get("traceparent")?.split("-")[1]).toBe(
      second.get("traceparent")?.split("-")[1],
    );
  });
});
