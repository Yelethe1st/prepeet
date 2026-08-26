import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ConnectionFailure, type LiveConnection } from "@/lib/rtc/connection";

import { LiveScreen } from "./LiveScreen";
import * as liveApi from "./api";
import { stashGrant } from "@/lib/rtc/grant";
import { resetGrantMemoryForTests } from "@/lib/rtc/grant";

const push = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, replace: push }),
}));

const connectLive = vi.fn();
vi.mock("./api");
vi.mock("@/lib/rtc/connection", async (importOriginal) => {
  const original =
    await importOriginal<typeof import("@/lib/rtc/connection")>();
  return {
    ...original,
    connectLive: (...args: unknown[]) => connectLive(...args),
  };
});

/**
 * The live route's connection lifecycle: RTC-01's boxes from the screen's
 * side. A missing grant and every failure kind land on a named explanation
 * with a way forward, never a spinner; leaving the screen ends the
 * connection, which is what releases the microphone.
 */

function liveConnection(): {
  connection: LiveConnection;
  endCalls: () => number;
} {
  let ends = 0;
  return {
    connection: {
      room: {} as LiveConnection["room"],
      end: async () => {
        ends++;
      },
    },
    endCalls: () => ends,
  };
}

function freshGrant(sessionId = "ses-1"): void {
  stashGrant({
    sessionId,
    url: "wss://rtc.test",
    room: sessionId,
    token: "tok",
    expiresAt: new Date(Date.now() + 60_000).toISOString(),
  });
}

afterEach(() => {
  sessionStorage.clear();
  resetGrantMemoryForTests();
  connectLive.mockReset();
  push.mockReset();
});

describe("joining", () => {
  it("connects with the stashed grant and says the interview is live", async () => {
    freshGrant();
    const { connection } = liveConnection();
    connectLive.mockResolvedValue(connection);

    render(<LiveScreen sessionId="ses-1" />);

    expect(await screen.findByText(/you are live/i)).toBeInTheDocument();
    expect(connectLive).toHaveBeenCalledWith(
      expect.objectContaining({ url: "wss://rtc.test", token: "tok" }),
      expect.anything(),
    );
    expect(screen.getByText(/recording is on/i)).toBeInTheDocument();
  });

  it("a missing grant is a named path back, not a spinner", async () => {
    render(<LiveScreen sessionId="ses-1" />);

    expect(
      await screen.findByText(/no live pass for this session/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /back to the prepare screen/i }),
    ).toHaveAttribute("href", "/session/ses-1/prepare");
    expect(connectLive).not.toHaveBeenCalled();
  });
});

describe("failures are named with recovery steps", () => {
  it.each([
    ["unauthorized", /pass has expired/i, /press start again/i],
    ["microphone", /microphone/i, /allow the microphone/i],
    ["unreachable", /could not reach/i, /connection|network/i],
  ] as const)("%s", async (kind, what, recovery) => {
    freshGrant();
    connectLive.mockRejectedValue(new ConnectionFailure(kind, new Error("x")));

    render(<LiveScreen sessionId="ses-1" />);

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(what);
    expect(screen.getByText(recovery)).toBeInTheDocument();
    expect(screen.queryByText(/you are live/i)).not.toBeInTheDocument();
  });
});

describe("leaving releases everything", () => {
  it("the end button ends the connection, seals at the cursor, and lands on the receipt", async () => {
    freshGrant();
    const { connection, endCalls } = liveConnection();
    connectLive.mockResolvedValue(connection);
    vi.mocked(liveApi.getInterview).mockResolvedValue({
      cursor: { connection_epoch: 1, accepted_sequence: 7 },
    } as Awaited<ReturnType<typeof liveApi.getInterview>>);
    vi.mocked(liveApi.completeInterview).mockResolvedValue(
      {} as Awaited<ReturnType<typeof liveApi.completeInterview>>,
    );
    const user = userEvent.setup();

    render(<LiveScreen sessionId="ses-1" />);
    await screen.findByText(/you are live/i);
    await user.click(screen.getByRole("button", { name: /end interview/i }));

    await waitFor(() => expect(endCalls()).toBe(1));
    await waitFor(() =>
      expect(liveApi.completeInterview).toHaveBeenCalledWith("ses-1", 1, 7),
    );
    expect(push).toHaveBeenCalledWith("/session/ses-1/complete");
  });

  it("unmounting ends the connection: navigation never leaves the microphone open", async () => {
    freshGrant();
    const { connection, endCalls } = liveConnection();
    connectLive.mockResolvedValue(connection);

    const { unmount } = render(<LiveScreen sessionId="ses-1" />);
    await screen.findByText(/you are live/i);
    unmount();

    await waitFor(() => expect(endCalls()).toBe(1));
  });
});
