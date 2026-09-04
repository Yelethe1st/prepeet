"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/shared/components";
import { ApiError } from "@/lib/api/client";
import {
  ConnectionFailure,
  connectLive,
  type LiveConnection,
} from "@/lib/rtc/connection";
import { consumeGrant } from "@/lib/rtc/grant";
import { Recovery, type RecoveryPhase } from "@/lib/rtc/recovery";
import { claimLiveTab, type Claim } from "@/lib/rtc/tab-lock";
import { Timeline } from "@/lib/rtc/timeline";

import { ReconnectionOverlay } from "./ReconnectionOverlay";
import {
  completeInterview,
  getInterview,
  resumeInterview,
  sendEvents,
} from "./api";

/**
 * The live route's connection shell - RTC-01's joining and RTC-03's
 * recovery. One tab holds the session; joining opens the control timeline
 * and tells the server the connection established; an unexpected drop runs
 * the recovery chain under the ported overlay; and a refresh or an expired
 * grant resumes into the same session rather than bouncing to prepare,
 * because the session is the server's and losing a tab loses nothing.
 *
 * Every way this can end is a named explanation with a way forward. The
 * ticket's old law still holds: never a spinner with no words.
 */

type Phase =
  | { kind: "connecting" }
  | { kind: "live" }
  | { kind: "occupied" }
  | { kind: "no-grant" }
  | { kind: "failed"; failure: ConnectionFailure["kind"] }
  | { kind: "ended" }
  | { kind: "expired" }
  | { kind: "superseded" }
  | { kind: "unresumable" };

export function LiveScreen({ sessionId }: { sessionId: string }) {
  const router = useRouter();
  const [phase, setPhase] = useState<Phase>({ kind: "connecting" });
  const [recovering, setRecovering] = useState<Extract<
    RecoveryPhase,
    { kind: "reconnecting" | "exhausted" }
  > | null>(null);
  const [recovered, setRecovered] = useState(false);

  const connection = useRef<LiveConnection | null>(null);
  const timeline = useRef<Timeline | null>(null);
  const recovery = useRef<Recovery | null>(null);
  const claim = useRef<Claim | null>(null);

  useEffect(() => {
    let cancelled = false;

    const connect = async (grant: {
      url: string;
      token: string;
    }): Promise<void> => {
      const live = await connectLive(grant, {
        onEnded: () => {
          if (!cancelled) {
            setPhase({ kind: "ended" });
          }
        },
        onDropped: () => {
          if (!cancelled) {
            setRecovered(false);
            recovery.current?.begin("connection_lost");
          }
        },
      });
      if (cancelled) {
        void live.end();
        throw new ConnectionFailure("unreachable", new Error("cancelled"));
      }
      connection.current = live;
    };

    // Opens the control timeline at the epoch the server says this
    // connection speaks, wires the recovery chain to it, and delivers the
    // established event that moves the interview to running.
    const open = (epoch: number): void => {
      const line = new Timeline({
        post: (forEpoch, events) => sendEvents(sessionId, forEpoch, events),
        epoch,
      });
      timeline.current = line;
      recovery.current = new Recovery({
        resume: async () => {
          const resumed = await resumeInterview(sessionId);
          return {
            grant: {
              url: resumed.realtime.url,
              token: resumed.realtime.token,
            },
            epoch: resumed.session.cursor?.connection_epoch ?? epoch + 1,
            previousAccepted: resumed.recovery.accepted_sequence,
          };
        },
        reconnect: connect,
        timeline: line,
        refusalCode: (error) => (error instanceof ApiError ? error.code : ""),
        onPhase: (next) => {
          if (cancelled) {
            return;
          }
          switch (next.kind) {
            case "reconnecting":
            case "exhausted":
              setRecovering(next);
              return;
            case "recovered":
              setRecovering(null);
              setRecovered(true);
              return;
            case "expired":
              setRecovering(null);
              setPhase({ kind: "expired" });
              return;
            case "superseded":
              setRecovering(null);
              setPhase({ kind: "superseded" });
              return;
            case "unresumable":
              setRecovering(null);
              setPhase({ kind: "unresumable" });
              return;
          }
        },
      });
      line.record("connection.established");
      void line.flush().catch(() => undefined);
    };

    const joinWithGrant = async (grant: {
      url: string;
      token: string;
    }): Promise<void> => {
      await connect(grant);
      // The epoch this connection speaks comes from the session, not from
      // anything remembered locally: the server's record is the record. A
      // read that fails does not block joining; start opens epoch one.
      let epoch = 1;
      try {
        const session = await getInterview(sessionId);
        epoch = session?.cursor?.connection_epoch ?? 1;
      } catch {
        // The default stands.
      }
      open(epoch);
      setPhase({ kind: "live" });
    };

    // No stashed grant: a refresh, a restored laptop, an expired pass. The
    // session is still the server's, so resume is the front door back in.
    const joinByResume = async (): Promise<void> => {
      let resumed: Awaited<ReturnType<typeof resumeInterview>>;
      try {
        resumed = await resumeInterview(sessionId);
      } catch (error) {
        const code = error instanceof ApiError ? error.code : "";
        if (code === "SESSION_NOT_RESUMABLE") {
          setPhase({ kind: "no-grant" });
        } else if (code === "GRACE_EXPIRED") {
          setPhase({ kind: "expired" });
        } else if (code === "EPOCH_STALE") {
          setPhase({ kind: "superseded" });
        } else {
          setPhase({ kind: "failed", failure: "unreachable" });
        }
        return;
      }
      await connect({
        url: resumed.realtime.url,
        token: resumed.realtime.token,
      });
      open(resumed.session.cursor?.connection_epoch ?? 1);
      setPhase({ kind: "live" });
    };

    const mount = async (): Promise<void> => {
      // One session, one live tab: ask before joining, and refuse rather
      // than supersede an interview mid-sentence in another tab.
      const held = await claimLiveTab(sessionId);
      if (cancelled) {
        held.release();
        return;
      }
      claim.current = held;
      if (!held.granted) {
        setPhase({ kind: "occupied" });
        return;
      }

      const grant = consumeGrant(sessionId);
      try {
        if (grant) {
          await joinWithGrant({ url: grant.url, token: grant.token });
        } else {
          await joinByResume();
        }
      } catch (error) {
        if (!cancelled) {
          setPhase({
            kind: "failed",
            failure:
              error instanceof ConnectionFailure ? error.kind : "unreachable",
          });
        }
      }
    };

    void mount();

    return () => {
      cancelled = true;
      recovery.current?.cancel();
      claim.current?.release();
      claim.current = null;
      // Navigation away is a teardown like any other: the microphone does
      // not survive the component.
      void connection.current?.end();
      connection.current = null;
    };
  }, [sessionId]);

  const endInterview = async (): Promise<void> => {
    recovery.current?.cancel();
    setRecovering(null);
    // The goodbye is a durable event; best effort, because leaving must
    // work from a broken connection too.
    timeline.current?.record("session.leave");
    await timeline.current?.flush().catch(() => undefined);
    await connection.current?.end();
    connection.current = null;
    // Seal at wherever the durable timeline stands, then hand over to the
    // receipt. Completion is idempotent to the receipt, so a retry after a
    // flaky request converges. A session that never established (no
    // cursor) has nothing to seal; practice is the honest landing.
    try {
      const session = await getInterview(sessionId);
      if (session.cursor) {
        await completeInterview(
          sessionId,
          session.cursor.connection_epoch,
          session.cursor.accepted_sequence,
        );
      }
      router.push(`/session/${sessionId}/complete`);
    } catch {
      router.push(`/session/${sessionId}/complete`);
    }
  };

  if (phase.kind === "connecting") {
    return (
      <p role="status" className="text-sm text-fg-2">
        Connecting to your interview. Nothing is recorded until you are in.
      </p>
    );
  }

  if (phase.kind === "occupied") {
    return (
      <div role="alert" className="max-w-[480px] space-y-2">
        <h2 className="text-base font-semibold">
          This interview is open in another tab
        </h2>
        <p className="text-sm text-fg-2">
          One connection holds an interview at a time, so nothing you say
          arrives twice. Continue in the tab that has it; this one can be
          closed.
        </p>
      </div>
    );
  }

  if (phase.kind === "no-grant") {
    return (
      <div role="alert" className="max-w-[480px] space-y-2">
        <h2 className="text-base font-semibold">
          There is no live pass for this session
        </h2>
        <p className="text-sm text-fg-2">
          A pass is issued when you press start and works for one join. Nothing
          about your session is lost.
        </p>
        <a
          className="text-sm font-semibold text-primary underline"
          href={`/session/${sessionId}/prepare`}
        >
          Back to the prepare screen
        </a>
      </div>
    );
  }

  if (phase.kind === "expired") {
    return (
      <div role="alert" className="max-w-[480px] space-y-2">
        <h2 className="text-base font-semibold">
          The reconnection window has passed
        </h2>
        <p className="text-sm text-fg-2">
          Everything you said before the connection dropped was kept and is
          being finalized. The interruption is recorded as an interruption,
          never as a poor answer, and what happens next is shown with your
          session.
        </p>
        <a
          className="text-sm font-semibold text-primary underline"
          href={`/session/${sessionId}/complete`}
        >
          See where your session stands
        </a>
      </div>
    );
  }

  if (phase.kind === "superseded") {
    return (
      <div role="alert" className="max-w-[480px] space-y-2">
        <h2 className="text-base font-semibold">
          Another connection took over this interview
        </h2>
        <p className="text-sm text-fg-2">
          The interview continues where it was resumed, and nothing said here
          after the takeover was recorded twice. If that was not you, end the
          interview from the device that has it.
        </p>
      </div>
    );
  }

  if (phase.kind === "unresumable") {
    return (
      <div role="alert" className="max-w-[480px] space-y-2">
        <h2 className="text-base font-semibold">
          This interview is not running any more
        </h2>
        <p className="text-sm text-fg-2">
          It may have finished or been finalized while this tab was away.
          Everything you said in it was kept.
        </p>
        <a
          className="text-sm font-semibold text-primary underline"
          href={`/session/${sessionId}/complete`}
        >
          See where your session stands
        </a>
      </div>
    );
  }

  if (phase.kind === "failed") {
    return <Failed failure={phase.failure} sessionId={sessionId} />;
  }

  if (phase.kind === "ended") {
    return (
      <div className="max-w-[480px] space-y-2">
        <h2 className="text-base font-semibold">The connection has ended</h2>
        <p className="text-sm text-fg-2">
          Your microphone is off. Everything you said was kept.
        </p>
      </div>
    );
  }

  return (
    <div className="max-w-[560px] space-y-4">
      <p className="rounded-md border border-success-border bg-success-soft px-4 py-3 text-sm text-success-fg">
        You are live. Recording is on, exactly as you consented on the prepare
        screen.
      </p>
      {recovered ? (
        <p role="status" className="text-sm text-fg-2">
          Connection restored. Everything you said before the drop was already
          recorded; the interview picks up where it left off.
        </p>
      ) : null}
      <p className="text-sm text-fg-2">
        The interviewer joins here as the live surface is built out. Your
        microphone releases the moment you end or leave, whichever comes first.
      </p>
      <Button
        type="button"
        variant="secondary"
        onClick={() => void endInterview()}
      >
        End interview
      </Button>
      {recovering ? (
        <ReconnectionOverlay
          phase={recovering}
          onRetryNow={() => recovery.current?.retryNow()}
          onEndInterview={() => void endInterview()}
        />
      ) : null}
    </div>
  );
}

function Failed({
  failure,
  sessionId,
}: {
  failure: ConnectionFailure["kind"];
  sessionId: string;
}) {
  const named: Record<
    ConnectionFailure["kind"],
    { what: string; recovery: string }
  > = {
    unauthorized: {
      what: "Your live pass has expired",
      recovery:
        "Passes are short-lived on purpose. Go back to the prepare screen and press start again; nothing you set up is lost.",
    },
    microphone: {
      what: "The microphone could not be opened",
      recovery:
        "Allow the microphone for this site in your browser's permissions, then return to the prepare screen; its device check will confirm the fix.",
    },
    unreachable: {
      what: "We could not reach the interview server",
      recovery:
        "Check your connection and try again from the prepare screen. If your network blocks WebRTC, a different network is the fastest fix.",
    },
  };
  const message = named[failure];

  return (
    <div role="alert" className="max-w-[480px] space-y-2">
      <h2 className="text-base font-semibold">{message.what}</h2>
      <p className="text-sm text-fg-2">{message.recovery}</p>
      <p className="text-sm text-fg-2">Nothing was recorded.</p>
      <a
        className="text-sm font-semibold text-primary underline"
        href={`/session/${sessionId}/prepare`}
      >
        Back to the prepare screen
      </a>
    </div>
  );
}
