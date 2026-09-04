import { Room } from "livekit-client";

/**
 * The browser's side of ADR-0012's topology: connect to the SFU with the
 * grant SES-02 minted, open the microphone, and guarantee the release.
 *
 * The one law here is the ticket's second box: teardown always releases the
 * microphone, on navigation, close and error alike. Every path out of a
 * connection - explicit end, component unmount, tab close, connect failure,
 * microphone refusal, the server dropping us - funnels into a single
 * idempotent teardown, because two cleanup paths is one that will be
 * forgotten.
 */

/** The structural surface the wrapper needs; livekit-client's Room has it. */
export interface RoomLike {
  connect(url: string, token: string): Promise<void>;
  disconnect(): Promise<void>;
  localParticipant: {
    setMicrophoneEnabled(enabled: boolean): Promise<unknown>;
  };
  on(event: string, handler: () => void): unknown;
  off(event: string, handler: () => void): unknown;
}

/**
 * Why a connection could not be had, as a name the screen can offer
 * recovery for. A spinner is not an answer; the third box forbids it.
 */
export type FailureKind = "unauthorized" | "microphone" | "unreachable";

export class ConnectionFailure extends Error {
  readonly kind: FailureKind;

  constructor(kind: FailureKind, cause: unknown) {
    super(`live connection failed: ${kind}`);
    this.name = "ConnectionFailure";
    this.kind = kind;
    this.cause = cause;
  }
}

export interface LiveConnection {
  room: RoomLike;
  /** Idempotent teardown: disconnects and releases the microphone. */
  end(): Promise<void>;
}

export interface ConnectOptions {
  /** Swappable for tests; defaults to livekit-client's Room. */
  createRoom?: () => RoomLike;
  /** Called once when the connection is over, whoever ended it. */
  onEnded?: () => void;
  /**
   * Called instead of onEnded when the server drops the connection and the
   * caller can recover: RTC-03's chain resumes into a fresh connection, so
   * an unexpected drop is a recovery trigger rather than an ending. A
   * deliberate end never lands here.
   */
  onDropped?: () => void;
}

export async function connectLive(
  grant: { url: string; token: string },
  options: ConnectOptions = {},
): Promise<LiveConnection> {
  const room = options.createRoom?.() ?? (new Room() as unknown as RoomLike);

  let ended = false;
  const end = async (): Promise<void> => {
    if (ended) {
      return;
    }
    ended = true;
    window.removeEventListener("pagehide", onPageHide);
    room.off("disconnected", onServerDisconnect);
    try {
      // disconnect stops and releases every local track; the microphone
      // indicator goes off with it.
      await room.disconnect();
    } finally {
      options.onEnded?.();
    }
  };

  // The tab closing mid-answer is the case people forget: pagehide is the
  // event that still fires then, and the browser gives it enough time to
  // let the tracks go.
  const onPageHide = (): void => {
    void end();
  };
  const onServerDisconnect = (): void => {
    if (!ended) {
      ended = true;
      window.removeEventListener("pagehide", onPageHide);
      room.off("disconnected", onServerDisconnect);
      if (options.onDropped) {
        options.onDropped();
      } else {
        options.onEnded?.();
      }
    }
  };

  try {
    await room.connect(grant.url, grant.token);
  } catch (cause) {
    await room.disconnect().catch(() => undefined);
    throw new ConnectionFailure(classify(cause), cause);
  }

  try {
    await room.localParticipant.setMicrophoneEnabled(true);
  } catch (cause) {
    // A connection nobody can speak into must not sit there looking alive.
    await room.disconnect().catch(() => undefined);
    throw new ConnectionFailure("microphone", cause);
  }

  window.addEventListener("pagehide", onPageHide);
  room.on("disconnected", onServerDisconnect);

  return { room, end };
}

function classify(cause: unknown): FailureKind {
  const text = cause instanceof Error ? cause.message.toLowerCase() : "";
  if (
    text.includes("token") ||
    text.includes("unauthorized") ||
    text.includes("permission")
  ) {
    return "unauthorized";
  }
  return "unreachable";
}
