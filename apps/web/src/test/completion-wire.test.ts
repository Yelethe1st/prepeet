import { beforeEach, describe, expect, it, vi } from "vitest";

import { completeInterview as completeFromComplete } from "@/features/complete/api";
import { completeInterview as completeFromLive } from "@/features/live/api";

/**
 * What actually goes on the wire, through the real apiFetch.
 *
 * The existing suites for these two modules mock apiFetch and assert the value
 * handed to it, which is why they were green while every completion failed:
 * both callers passed an already-serialised string and apiFetch serialises
 * whatever it is given, so the server received a JSON *string* where the
 * decoder wanted an object. It answered 400, the session was never sealed, and
 * LiveScreen navigated to the completion page regardless.
 *
 * A mock at the boundary you are testing cannot see a bug on the other side of
 * it. These stub fetch instead and read the body the server would have parsed.
 *
 * It lives in src/test rather than in either feature because it reads both,
 * and a feature importing another feature is what the boundary test forbids:
 * it caught this file in its first home, which is the rule working.
 */

const fetchMock = vi.fn();

beforeEach(() => {
  fetchMock.mockReset();
  fetchMock.mockResolvedValue(
    new Response(JSON.stringify({}), {
      status: 200,
      headers: { "content-type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);
});

function sentBody(): unknown {
  const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
  return JSON.parse(init.body as string);
}

describe("completing an interview", () => {
  for (const [name, complete] of [
    ["from the live screen", completeFromLive],
    ["from the completion screen", completeFromComplete],
  ] as const) {
    it(`${name} sends an object the server can decode`, async () => {
      await complete("ses-6", 1, 4);

      // An object, not a string. A string here is what the Go decoder refuses
      // with a 400, and it is indistinguishable from success on this side.
      expect(sentBody()).toEqual({ connection_epoch: 1, final_sequence: 4 });
      expect(typeof sentBody()).toBe("object");
    });

    it(`${name} posts to the completion route`, async () => {
      await complete("ses-6", 1, 4);

      const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
      expect(url).toContain("/api/v1/interviews/ses-6/complete");
      expect(init.method).toBe("POST");
    });
  }
});
