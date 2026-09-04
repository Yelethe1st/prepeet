import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api/client";
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

function refused(code: string): ApiError {
  return new ApiError({ status: 409, code, message: code });
}

afterEach(() => {
  sessionStorage.clear();
  resetGrantMemoryForTests();
  connectLive.mockReset();
  push.mockReset();
  vi.mocked(liveApi.resumeInterview).mockReset();
  vi.mocked(liveApi.sendEvents).mockReset();
  vi.mocked(liveApi.getInterview).mockReset();
  vi.mocked(liveApi.completeInterview).mockReset();
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

  it("a missing grant on a session that never started is a named path back, not a spinner", async () => {
    // Resume is the front door back in; only its refusal says the session
    // truly has nothing running to rejoin.
    vi.mocked(liveApi.resumeInterview).mockRejectedValue(
      refused("SESSION_NOT_RESUMABLE"),
    );

    render(<LiveScreen sessionId="ses-1" />);

    expect(
      await screen.findByText(/no live pass for this session/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /back to the prepare screen/i }),
    ).toHaveAttribute("href", "/session/ses-1/prepare");
    expect(connectLive).not.toHaveBeenCalled();
  });

  it("a refresh resumes into the same session instead of bouncing to prepare", async () => {
    const { connection } = liveConnection();
    connectLive.mockResolvedValue(connection);
    vi.mocked(liveApi.resumeInterview).mockResolvedValue({
      session: { cursor: { connection_epoch: 2, accepted_sequence: 3 } },
      realtime: { url: "wss://rtc.test", token: "tok-2" },
      recovery: { previous_epoch: 1, accepted_sequence: 3, missing: [] },
    } as never);

    render(<LiveScreen sessionId="ses-1" />);

    expect(await screen.findByText(/you are live/i)).toBeInTheDocument();
    expect(connectLive).toHaveBeenCalledWith(
      expect.objectContaining({ url: "wss://rtc.test", token: "tok-2" }),
      expect.anything(),
    );
  });

  it("a lapsed reconnection window at mount says what happened and what happens next", async () => {
    vi.mocked(liveApi.resumeInterview).mockRejectedValue(
      refused("GRACE_EXPIRED"),
    );

    render(<LiveScreen sessionId="ses-1" />);

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/reconnection window has passed/i);
    expect(alert).toHaveTextContent(/kept and is being finalized/i);
    expect(
      screen.getByRole("link", { name: /where your session stands/i }),
    ).toHaveAttribute("href", "/session/ses-1/complete");
  });

  it("a duplicate tab is refused rather than producing two live connections", async () => {
    freshGrant();
    const { connection } = liveConnection();
    connectLive.mockResolvedValue(connection);

    render(<LiveScreen sessionId="ses-1" />);
    await screen.findByText(/you are live/i);

    render(<LiveScreen sessionId="ses-1" />);

    expect(await screen.findByText(/open in another tab/i)).toBeInTheDocument();
    // One connection: the second tab never joined.
    expect(connectLive).toHaveBeenCalledTimes(1);
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

describe("the room ending on its own", () => {
  it("shows the ended state when the connection reports it, not a stuck live screen", async () => {
    freshGrant();
    const { connection } = liveConnection();
    // The wrapper hands the screen its own onEnded; capture and fire it,
    // which is what a deliberate end elsewhere does.
    let ended: (() => void) | undefined;
    connectLive.mockImplementation(
      async (_grant: unknown, handlers: { onEnded: () => void }) => {
        ended = handlers.onEnded;
        return connection;
      },
    );

    render(<LiveScreen sessionId="ses-1" />);
    await screen.findByText(/you are live/i);

    act(() => ended?.());

    expect(
      await screen.findByText(/connection has ended/i),
    ).toBeInTheDocument();
  });
});

describe("an unexpected drop runs the recovery chain", () => {
  function dropHarness(): {
    connection: LiveConnection;
    drop: () => void;
  } {
    const { connection } = liveConnection();
    let dropped: (() => void) | undefined;
    connectLive.mockImplementation(
      async (_grant: unknown, handlers: { onDropped: () => void }) => {
        dropped = handlers.onDropped;
        return connection;
      },
    );
    return { connection, drop: () => dropped?.() };
  }

  it("shows the ported overlay and announces the attempt to assistive technology", async () => {
    freshGrant();
    const { drop } = dropHarness();
    // The chain stays in its first attempt: resume does not answer yet.
    vi.mocked(liveApi.resumeInterview).mockImplementation(
      () => new Promise(() => undefined),
    );

    render(<LiveScreen sessionId="ses-1" />);
    await screen.findByText(/you are live/i);

    act(() => drop());

    const overlay = await screen.findByRole("alertdialog", {
      name: /reconnecting to the interview/i,
    });
    expect(overlay).toHaveTextContent(/interview is paused/i);
    expect(overlay).toHaveTextContent(/safely recorded/i);
    // The attempt counter is a live region: each change is announced
    // without the dialog re-announcing itself.
    expect(screen.getByRole("status", { name: "" }).textContent).toMatch(
      /reconnection attempt 1 of 5/i,
    );
  });

  it("retry now runs the chain again and recovery is announced", async () => {
    freshGrant();
    const { drop } = dropHarness();
    vi.mocked(liveApi.sendEvents).mockResolvedValue({
      accepted_sequence: 0,
      outcomes: [],
    });
    vi.mocked(liveApi.resumeInterview)
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValue({
        session: { cursor: { connection_epoch: 2, accepted_sequence: 0 } },
        realtime: { url: "wss://rtc.test", token: "tok-2" },
        recovery: { previous_epoch: 1, accepted_sequence: 0, missing: [] },
      } as never);
    const user = userEvent.setup();

    render(<LiveScreen sessionId="ses-1" />);
    await screen.findByText(/you are live/i);

    act(() => drop());
    await screen.findByRole("alertdialog");

    await user.click(screen.getByRole("button", { name: /retry now/i }));

    await waitFor(() =>
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument(),
    );
    expect(screen.getByText(/connection restored/i)).toBeInTheDocument();
  });

  it("a takeover by another connection is a named ending, not a retry loop", async () => {
    freshGrant();
    const { drop } = dropHarness();
    vi.mocked(liveApi.resumeInterview).mockRejectedValue(
      refused("EPOCH_STALE"),
    );

    render(<LiveScreen sessionId="ses-1" />);
    await screen.findByText(/you are live/i);

    act(() => drop());

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/another connection took over/i);
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("a window that lapses mid-recovery lands on the expired explanation", async () => {
    freshGrant();
    const { drop } = dropHarness();
    vi.mocked(liveApi.resumeInterview).mockRejectedValue(
      refused("GRACE_EXPIRED"),
    );

    render(<LiveScreen sessionId="ses-1" />);
    await screen.findByText(/you are live/i);

    act(() => drop());

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/reconnection window has passed/i);
  });
});
