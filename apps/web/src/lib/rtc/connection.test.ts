import { afterEach, describe, expect, it, vi } from "vitest";

import { ConnectionFailure, connectLive, type RoomLike } from "./connection";

/**
 * The connection wrapper's one law: the microphone is released every time,
 * on navigation, close and error alike. Each test constructs a way for
 * teardown to be forgotten and asserts it was not. The real Room comes from
 * livekit-client; these fakes implement the same structural surface, so the
 * wrapper's obligations are pinned independently of the SDK.
 */

class FakeRoom implements RoomLike {
  connected = false;
  micEnabled = false;
  disconnects = 0;
  handlers = new Map<string, Set<() => void>>();

  connectError: Error | null = null;
  micError: Error | null = null;

  async connect(): Promise<void> {
    if (this.connectError) {
      throw this.connectError;
    }
    this.connected = true;
  }

  async disconnect(): Promise<void> {
    this.connected = false;
    this.micEnabled = false;
    this.disconnects++;
    this.emit("disconnected");
  }

  localParticipant = {
    setMicrophoneEnabled: async (enabled: boolean) => {
      if (enabled && this.micError) {
        throw this.micError;
      }
      this.micEnabled = enabled;
      return undefined;
    },
  };

  on(event: string, handler: () => void): this {
    const set = this.handlers.get(event) ?? new Set();
    set.add(handler);
    this.handlers.set(event, set);
    return this;
  }

  off(event: string, handler: () => void): this {
    this.handlers.get(event)?.delete(handler);
    return this;
  }

  emit(event: string): void {
    for (const handler of this.handlers.get(event) ?? []) {
      handler();
    }
  }
}

const grant = { url: "wss://rtc.test", token: "tok" };

afterEach(() => {
  vi.restoreAllMocks();
});

describe("connecting", () => {
  it("connects and opens the microphone", async () => {
    const room = new FakeRoom();
    const live = await connectLive(grant, { createRoom: () => room });

    expect(room.connected).toBe(true);
    expect(room.micEnabled).toBe(true);
    await live.end();
  });

  it("an unreachable SFU is a named failure, and nothing is left holding media", async () => {
    const room = new FakeRoom();
    room.connectError = new Error("could not establish pc connection");

    await expect(
      connectLive(grant, { createRoom: () => room }),
    ).rejects.toSatisfy(
      (error: unknown) =>
        error instanceof ConnectionFailure && error.kind === "unreachable",
    );
    expect(room.micEnabled).toBe(false);
    expect(room.disconnects).toBeGreaterThan(0);
  });

  it("a refused microphone tears the connection down rather than sitting half-open", async () => {
    const room = new FakeRoom();
    room.micError = new Error("Permission denied");

    await expect(
      connectLive(grant, { createRoom: () => room }),
    ).rejects.toSatisfy(
      (error: unknown) =>
        error instanceof ConnectionFailure && error.kind === "microphone",
    );
    // The room was connected, then let go: a session nobody can speak into
    // must not keep a connection that looks alive.
    expect(room.connected).toBe(false);
  });

  it("a rejected token is named as authorization, not as a network mystery", async () => {
    const room = new FakeRoom();
    room.connectError = new Error("invalid token: expired");

    await expect(
      connectLive(grant, { createRoom: () => room }),
    ).rejects.toSatisfy(
      (error: unknown) =>
        error instanceof ConnectionFailure && error.kind === "unauthorized",
    );
  });
});

describe("teardown always releases the microphone", () => {
  it("on end, idempotently", async () => {
    const room = new FakeRoom();
    const live = await connectLive(grant, { createRoom: () => room });

    await live.end();
    await live.end();

    expect(room.micEnabled).toBe(false);
    expect(room.disconnects).toBe(1);
  });

  it("on tab close: pagehide fires the same teardown", async () => {
    const room = new FakeRoom();
    await connectLive(grant, { createRoom: () => room });

    window.dispatchEvent(new Event("pagehide"));

    expect(room.micEnabled).toBe(false);
    expect(room.disconnects).toBe(1);
  });

  it("after end, the pagehide listener is gone, so nothing double-fires later", async () => {
    const room = new FakeRoom();
    const live = await connectLive(grant, { createRoom: () => room });
    await live.end();

    window.dispatchEvent(new Event("pagehide"));

    expect(room.disconnects).toBe(1);
  });

  it("a server-side disconnect still runs local cleanup", async () => {
    const room = new FakeRoom();
    const ended = vi.fn();
    const live = await connectLive(grant, {
      createRoom: () => room,
      onEnded: ended,
    });

    room.emit("disconnected");

    expect(ended).toHaveBeenCalledTimes(1);
    // And ending again after the server already dropped us is quiet.
    await live.end();
  });
});
