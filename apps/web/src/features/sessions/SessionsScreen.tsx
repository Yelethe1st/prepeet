"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { ApiError } from "@/lib/api/client";
import {
  EmptyState,
  ErrorState,
  LoadingSurface,
  SkeletonBlock,
  SkeletonText,
} from "@/shared/states";

import { listSessions, type InterviewSession } from "./api";
import { STATE_ROWS, type FilterGroup, type MachineState } from "./states";

/**
 * The session history - SES-07, from the prototype's candidate-sessions
 * screen with the recorded gap fixed: every state the machine can reach
 * has a row vocabulary (states.ts) and the completeness test walks the
 * machine's own list, so "abandoned"-style invented states and hidden
 * expired rows are both structurally impossible.
 *
 * The filter is the URL's: read from the query string, written back on
 * change, so a refresh or a shared link keeps it.
 */

const FILTERS: { key: FilterGroup | "all"; label: string }[] = [
  { key: "all", label: "All" },
  { key: "active", label: "In motion" },
  { key: "finished", label: "Finished" },
  { key: "attention", label: "Needs attention" },
];

export function SessionsScreen() {
  // No silent retries: a history read that fails is told at once, with a
  // retry the person chooses, exactly as the other session screens do.
  const sessions = useQuery({
    queryKey: ["sessions"],
    queryFn: listSessions,
    retry: false,
  });
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const raw = params.get("filter");
  const filter: FilterGroup | "all" =
    raw === "active" || raw === "finished" || raw === "attention" ? raw : "all";

  if (sessions.isPending) {
    return (
      <LoadingSurface label="Loading your sessions">
        <SkeletonBlock className="h-[80px]" />
        <SkeletonText />
        <SkeletonBlock className="h-[80px]" />
      </LoadingSurface>
    );
  }
  if (sessions.error) {
    const failure =
      sessions.error instanceof ApiError ? sessions.error : undefined;
    return (
      <ErrorState
        what="Your session history could not be loaded"
        safe="The sessions themselves are unaffected."
        action={
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => void sessions.refetch()}
          >
            Try again
          </button>
        }
        reference={failure?.requestId ?? "no request id"}
      />
    );
  }

  if (sessions.data.length === 0) {
    return (
      <EmptyState
        title="No practice sessions yet"
        action={
          <Link className="btn btn-primary" href="/practice/new">
            Start a practice interview
          </Link>
        }
      >
        <p>
          Your history lives here: every session, whatever became of it, with
          the way onward from each.
        </p>
      </EmptyState>
    );
  }

  const shown = sessions.data.filter(
    (session) => filter === "all" || rowFor(session.state).group === filter,
  );

  return (
    <div className="space-y-4">
      <div role="tablist" aria-label="Filter sessions" className="flex gap-2">
        {FILTERS.map(({ key, label }) => (
          <button
            key={key}
            role="tab"
            aria-selected={filter === key}
            className={`rounded-full border border-border px-3 py-1 text-sm ${
              filter === key ? "bg-accent-soft font-semibold" : ""
            }`}
            onClick={() =>
              router.replace(
                key === "all" ? pathname : `${pathname}?filter=${key}`,
                { scroll: false },
              )
            }
          >
            {label}
          </button>
        ))}
      </div>

      {shown.length === 0 ? (
        <p className="text-sm text-fg-2" role="status">
          Nothing under this filter. Every session still exists under All.
        </p>
      ) : (
        <ul className="space-y-3">
          {shown.map((session) => (
            <SessionRow key={session.id} session={session} />
          ))}
        </ul>
      )}
    </div>
  );
}

/** One session: when, what, where it stands, and the action that applies. */
function SessionRow({ session }: { session: InterviewSession }) {
  const row = rowFor(session.state);
  return (
    <li
      data-testid={`session-${session.state}`}
      data-session-id={session.id}
      className="flex flex-wrap items-center gap-3 rounded-lg border border-border bg-surface px-4 py-3"
    >
      <div className="min-w-0 flex-1">
        <p className="text-sm font-semibold">
          {new Date(session.created_at).toLocaleDateString()} ·{" "}
          {session.config.minutes} minutes
        </p>
        <p className="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-fg-2">
          <span>{row.label}</span>
          {session.failure_code && (
            <span className="font-mono">{session.failure_code}</span>
          )}
        </p>
      </div>
      <Link className="btn btn-secondary btn-sm" href={row.href(session.id)}>
        {row.action}
      </Link>
    </li>
  );
}

/** The vocabulary row for a state; an unknown state reads as itself with
 * the safest action, rather than crashing history over a new word. */
function rowFor(state: string) {
  return (
    STATE_ROWS[state as MachineState] ?? {
      label: state,
      group: "attention" as const,
      action: "See status",
      href: (id: string) => `/session/${id}/complete`,
    }
  );
}
