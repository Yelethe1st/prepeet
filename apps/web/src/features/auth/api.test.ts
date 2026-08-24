import { beforeEach, describe, expect, it, vi } from "vitest";

import { currentUser, listMemberships, register, setActiveTenant, signIn, signOut } from "./api";

/**
 * The authentication calls.
 *
 * Small tests for a small file, and worth having because what they check is
 * exactly what a typo breaks: the path, the method, and whether a body is sent.
 * A wrong path here is a 404 that nothing catches until somebody tries the
 * screen, and the generated types cannot help because a path is a string.
 */

const fetchMock = vi.fn();

beforeEach(() => {
  fetchMock.mockReset();
  fetchMock.mockResolvedValue(
    new Response(JSON.stringify({}), { status: 200, headers: { "content-type": "application/json" } }),
  );
  vi.stubGlobal("fetch", fetchMock);
});

function lastCall(): [string, RequestInit] {
  return fetchMock.mock.calls[0] as [string, RequestInit];
}

describe("auth api", () => {
  it("signs in by posting credentials to the login route", async () => {
    await signIn({ email: "a@b.co", password: "x" });

    const [url, init] = lastCall();
    expect(url).toContain("/api/v1/auth/login");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ email: "a@b.co", password: "x" });
  });

  it("registers by posting to the register route", async () => {
    await register({ email: "a@b.co", password: "x", account_type: "candidate" });

    const [url, init] = lastCall();
    expect(url).toContain("/api/v1/auth/register");
    expect(init.method).toBe("POST");
  });

  it("carries the organisation name when there is one", async () => {
    await register({
      email: "a@b.co",
      password: "x",
      account_type: "organisation",
      organisation_name: "Northwind",
    });

    const [, init] = lastCall();
    expect(JSON.parse(init.body as string)).toMatchObject({
      account_type: "organisation",
      organisation_name: "Northwind",
    });
  });

  it("reads the current user with a GET, not a POST", async () => {
    await currentUser();

    const [url, init] = lastCall();
    expect(url).toContain("/api/v1/me");
    expect(init.method).toBeUndefined();
  });

  it("signs out by posting, so it cannot be triggered by a link or a prefetch", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));

    await signOut();

    const [url, init] = lastCall();
    expect(url).toContain("/api/v1/auth/logout");
    expect(init.method).toBe("POST");
  });

  /**
   * The tokens are set as HttpOnly cookies and never appear in the body, so
   * there is nothing here to store. A version of this that returned something
   * to keep would be a version somebody put in localStorage.
   */
  it("returns only what the server describes, with no token to store", async () => {
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({ user_id: "usr_1", expires_at: "2026-01-01T00:00:00Z", authenticated_at: "2026-01-01T00:00:00Z" }),
        { status: 200, headers: { "content-type": "application/json" } },
      ),
    );

    const session = await signIn({ email: "a@b.co", password: "x" });

    expect(session.user_id).toBe("usr_1");
    expect(JSON.stringify(session)).not.toMatch(/token/i);
  });
});

/**
 * The two calls the workspace switcher makes.
 *
 * Added because they were the only functions in this file nothing exercised,
 * and the aggregate hid it: the suite was well above its floor while these sat
 * at zero. What they check is what a typo breaks, which is the path and the
 * method.
 */
describe("workspace selection", () => {
  it("chooses a workspace with a PUT, since it replaces the selection", async () => {
    await setActiveTenant("t-northwind");

    const [url, init] = lastCall();
    expect(url).toContain("/api/v1/me/active-tenant");
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body as string)).toEqual({ tenant_id: "t-northwind" });
  });

  /**
   * Null clears the selection, which is how somebody leaves a workspace without
   * signing out. It must reach the server as null rather than as an omitted
   * field, which the server would read as no change.
   */
  it("clears the selection with an explicit null", async () => {
    await setActiveTenant(null);

    const [, init] = lastCall();
    expect(JSON.parse(init.body as string)).toEqual({ tenant_id: null });
  });

  it("lists memberships with a GET", async () => {
    await listMemberships();

    const [url, init] = lastCall();
    expect(url).toContain("/api/v1/me/memberships");
    expect(init.method).toBeUndefined();
  });
});
