import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError, apiFetch } from "./client";

/**
 * The browser's side of the contract.
 *
 * What is asserted here is the handling every call shares: credentials, the
 * error envelope, and what happens when the response is not what the contract
 * describes. Whether a particular endpoint returns the right thing is settled
 * on the server, against a real database, and repeating it here would be two
 * places to update and one that gets forgotten.
 */

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
