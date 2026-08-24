import { useQuery } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ApiError } from "./client";
import { QueryProvider } from "./QueryProvider";

/**
 * The retry policy, which is the only decision this component makes.
 *
 * It is worth a test because getting it wrong is invisible: a retried 401 still
 * ends on the same screen, just later and after three more requests, so nothing
 * fails and nobody notices except the person waiting.
 */

/** Renders one query under the provider and reports what it settled on. */
function renderQuery(fetcher: () => Promise<string>) {
  function Subject() {
    const query = useQuery({ queryKey: ["subject"], queryFn: fetcher, gcTime: 0 });
    return <p>{query.isError ? "failed" : (query.data ?? "pending")}</p>;
  }

  render(
    <QueryProvider>
      <Subject />
    </QueryProvider>,
  );
}

describe("QueryProvider", () => {
  it("does not retry a refusal, because the answer will not change", async () => {
    const fetcher = vi.fn().mockRejectedValue(new ApiError({ status: 403, message: "forbidden" }));
    renderQuery(fetcher);

    await screen.findByText("failed");
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("retries a server failure, because that one might", async () => {
    const fetcher = vi
      .fn()
      .mockRejectedValueOnce(new ApiError({ status: 503, message: "unavailable" }))
      .mockResolvedValue("recovered");
    renderQuery(fetcher);

    // The provider leaves retryDelay at TanStack's default, which backs off
    // exponentially from a second. These waits are that delay, not slack.
    await screen.findByText("recovered", undefined, { timeout: 3_000 });
    expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it("gives up on a server failure rather than retrying forever", async () => {
    const fetcher = vi.fn().mockRejectedValue(new ApiError({ status: 500, message: "broken" }));
    renderQuery(fetcher);

    await screen.findByText("failed", undefined, { timeout: 5_000 });
    expect(fetcher).toHaveBeenCalledTimes(3);
  });

  it("retries a transport failure, which arrives as no status at all", async () => {
    const fetcher = vi.fn().mockRejectedValueOnce(new TypeError("network")).mockResolvedValue("ok");
    renderQuery(fetcher);

    await screen.findByText("ok", undefined, { timeout: 3_000 });
    expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it("gives each render its own client, so a server cannot share one session's cache", async () => {
    const first = vi.fn().mockResolvedValue("first");
    renderQuery(first);
    await screen.findByText("first");

    const second = vi.fn().mockResolvedValue("second");
    renderQuery(second);

    await waitFor(() => expect(second).toHaveBeenCalled());
  });
});
