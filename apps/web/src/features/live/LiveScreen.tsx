"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/shared/components";
import {
  ConnectionFailure,
  connectLive,
  type LiveConnection,
} from "@/lib/rtc/connection";

import { consumeGrant } from "@/lib/rtc/grant";

import { completeInterview, getInterview } from "./api";

/**
 * The live route's connection shell - RTC-01. Joins the room with the grant
 * the prepare screen handed off, and guarantees the leaving: the end
 * button, navigation away and the connection wrapper's own tab-close
 * handler all funnel into one teardown, which is what releases the
 * microphone every time. The interview surface itself - captions, the
 * interviewer, push to talk - lands with RTC-06 on top of this shell.
 *
 * Every way this can fail is a named explanation with a way forward. The
 * third box forbids the alternative by name: never a spinner.
 */

type Phase =
  | { kind: "connecting" }
  | { kind: "live" }
  | { kind: "no-grant" }
  | { kind: "failed"; failure: ConnectionFailure["kind"] }
  | { kind: "ended" };

export function LiveScreen({ sessionId }: { sessionId: string }) {
  const router = useRouter();
  const [phase, setPhase] = useState<Phase>({ kind: "connecting" });
  const connection = useRef<LiveConnection | null>(null);

  useEffect(() => {
    let cancelled = false;
    const grant = consumeGrant(sessionId);
    if (!grant) {
      // Deferred a tick: setting state synchronously inside an effect can
      // cascade renders, and the recovery screen loses nothing by arriving
      // one microtask later.
      queueMicrotask(() => {
        if (!cancelled) {
          setPhase({ kind: "no-grant" });
        }
      });
      return () => {
        cancelled = true;
      };
    }

    void connectLive(
      { url: grant.url, token: grant.token },
      {
        onEnded: () => {
          if (!cancelled) {
            setPhase({ kind: "ended" });
          }
        },
      },
    )
      .then((live) => {
        if (cancelled) {
          void live.end();
          return;
        }
        connection.current = live;
        setPhase({ kind: "live" });
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          setPhase({
            kind: "failed",
            failure:
              error instanceof ConnectionFailure ? error.kind : "unreachable",
          });
        }
      });

    return () => {
      cancelled = true;
      // Navigation away is a teardown like any other: the microphone does
      // not survive the component.
      void connection.current?.end();
      connection.current = null;
    };
  }, [sessionId]);

  const endInterview = async (): Promise<void> => {
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

  if (phase.kind === "failed") {
    return <Failed failure={phase.failure} sessionId={sessionId} />;
  }

  if (phase.kind === "ended") {
    return (
      <div className="max-w-[480px] space-y-2">
        <h2 className="text-base font-semibold">The connection has ended</h2>
        <p className="text-sm text-fg-2">
          Your microphone is off. If this was not you, your progress is safe;
          reconnection arrives with the recovery flow.
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
