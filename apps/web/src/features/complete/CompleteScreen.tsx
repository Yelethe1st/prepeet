"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";

import {
  ErrorState,
  LoadingSurface,
  SkeletonBlock,
  SkeletonText,
} from "@/shared/states";
import { ApiError } from "@/lib/api/client";

import { getInterview, type InterviewSession } from "./api";

/**
 * The completion receipt and processing status - SES-08, from the
 * prototype's candidate-session-complete screen.
 *
 * Everything here is the server's durable session read, which is what
 * makes the receipt survive leaving and returning: there is no client
 * state worth keeping. The stages are the state machine's own, no
 * completion time is promised anywhere (a recorded deviation from the
 * prototype's "usually takes under a minute"), delayed and failed are
 * different states with different roles, and a failure names the code and
 * the reference to quote. Screening sessions cannot reach this screen
 * today: the API's practice-only enum is the guard, and the screening
 * confirmation view lands with the SCR epic.
 */
export function CompleteScreen({ sessionId }: { sessionId: string }) {
  const session = useQuery({
    queryKey: ["interview", sessionId],
    queryFn: () => getInterview(sessionId),
    // Readiness arrives from the workflow; poll while it runs.
    refetchInterval: (query) => {
      const state = query.state.data?.state;
      return state === "finalizing" || state === "evaluating" ? 2500 : false;
    },
  });

  if (session.isPending) {
    return (
      <LoadingSurface label="Loading the session's status">
        <SkeletonBlock className="h-[120px]" />
        <SkeletonText />
      </LoadingSurface>
    );
  }
  if (session.error) {
    const failure =
      session.error instanceof ApiError ? session.error : undefined;
    return (
      <ErrorState
        what="The session's status could not be loaded"
        safe="The session itself is unaffected; whatever it reached is stored."
        action={
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => void session.refetch()}
          >
            Try again
          </button>
        }
        reference={failure?.requestId ?? "no request id"}
      />
    );
  }

  return <StatusBody session={session.data} />;
}

function StatusBody({ session }: { session: InterviewSession }) {
  switch (session.state) {
    case "finalizing":
    case "evaluating":
      return <Processing session={session} />;
    case "review_ready":
    case "archived":
      return <Ready session={session} />;
    case "evaluation_failed":
    case "finalization_failed":
      return <Failed session={session} />;
    default:
      return (
        <section
          aria-label="Session still live"
          className="rounded-lg border border-border bg-surface p-5"
        >
          <h2 className="text-lg font-semibold">
            This session is still in progress
          </h2>
          <p className="mt-2 text-sm text-fg-2">
            The receipt appears here once the interview ends.
          </p>
          <Link
            className="btn btn-secondary mt-4 inline-block"
            href={`/session/${session.id}/live`}
          >
            Back to the interview
          </Link>
        </section>
      );
  }
}

/** The honest stages: the state machine's own, no clock promised. */
function Processing({ session }: { session: InterviewSession }) {
  const sealed = session.seal !== undefined && session.seal !== null;
  return (
    <section
      aria-label="Processing"
      role="status"
      className="rounded-lg border border-border bg-surface p-5"
    >
      <h2 className="text-lg font-semibold">
        {session.state === "finalizing"
          ? "Sealing the transcript"
          : "Your interview is being evaluated"}
      </h2>
      <ol className="mt-4 space-y-2 text-sm">
        <Stage done label="Interview finished" />
        <Stage done={sealed} label="Transcript sealed" />
        <Stage
          done={false}
          label="Evaluation running: evidence, then results"
        />
      </ol>
      <p className="mt-4 text-sm text-fg-2">
        It is safe to leave this page or close the tab; processing continues
        either way, and this page checks again on its own.
      </p>
      {sealed && <MediaLine session={session} />}
    </section>
  );
}

/** The durable receipt with the ways onward. */
function Ready({ session }: { session: InterviewSession }) {
  return (
    <section
      aria-label="Session complete"
      className="rounded-lg border border-border bg-surface p-5"
    >
      <h2 className="text-lg font-semibold">Your results are ready</h2>
      {session.seal && (
        <p className="mt-2 text-sm text-fg-2">
          Transcript sealed at{" "}
          {new Date(session.seal.sealed_at).toLocaleString()}.
        </p>
      )}
      <MediaLine session={session} />
      <div className="mt-4 flex flex-wrap gap-3">
        <Link
          className="btn btn-primary"
          href={`/session/${session.id}/results`}
        >
          Outcome and evidence
        </Link>
        <Link
          className="btn btn-secondary"
          href={`/session/${session.id}/review`}
        >
          Coaching review
        </Link>
      </div>
    </section>
  );
}

/** Terminal failure: the code, the safety, and the concrete next action. */
function Failed({ session }: { session: InterviewSession }) {
  return (
    <div
      role="alert"
      className="rounded-lg border border-border bg-surface p-5"
    >
      <h2 className="text-lg font-semibold">
        Evaluation failed for this session
      </h2>
      <p className="mt-2 text-sm text-fg-2">
        Your transcript and evidence are safe and stored; nothing you said was
        lost. The failure is on our side
        {session.failure_code ? (
          <>
            {" "}
            (code <span className="font-mono">{session.failure_code}</span>)
          </>
        ) : null}
        .
      </p>
      <p className="mt-2 text-sm text-fg-2">
        If this does not resolve on its own, contact support and quote session{" "}
        <span className="font-mono">{session.id}</span>.
      </p>
      <Link className="btn btn-secondary mt-4 inline-block" href="/practice">
        Back to practice
      </Link>
    </div>
  );
}

/** What happened to the recording, in the words of the choice made. */
function MediaLine({ session }: { session: InterviewSession }) {
  if (!session.seal) {
    return null;
  }
  switch (session.seal.media_status) {
    case "none_by_choice":
      return (
        <p className="mt-2 text-sm text-fg-2">
          No audio, by your choice: the transcript is kept, replay and delivery
          measurement are off for this session.
        </p>
      );
    case "missing":
      return (
        <p className="mt-2 text-sm text-fg-2">
          The recording did not arrive, so replay is unavailable for this
          session. The transcript, results and coaching are unaffected.
        </p>
      );
    default:
      return (
        <p className="mt-2 text-sm text-fg-2">
          The recording is stored and will power replay and delivery coaching.
        </p>
      );
  }
}

function Stage({ done, label }: { done: boolean; label: string }) {
  return (
    <li className="flex items-center gap-2">
      <span aria-hidden="true">{done ? "✓" : "…"}</span>
      <span className={done ? "" : "text-fg-2"}>{label}</span>
      <span className="sr-only">{done ? "done" : "pending"}</span>
    </li>
  );
}
