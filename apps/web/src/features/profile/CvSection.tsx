"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import type { ChangeEvent } from "react";

import { ApiError } from "@/lib/api/client";
import { Button } from "@/shared/components";
import {
  DelayedState,
  EmptyState,
  ErrorState,
  LoadingSurface,
  SkeletonBlock,
  SkeletonText,
  UnassessableState,
} from "@/shared/states";

import { listDocuments, listFacts, reviewFact, uploadCv } from "./api";
import {
  correctionFor,
  currentDocument,
  factText,
  spanSentence,
  statusLabel,
  type Fact,
} from "./facts";

/**
 * The CV and what extraction read from it - PRO-04's review surface.
 *
 * The stance the whole section takes: extraction is assistive, never
 * authoritative. Every fact shows exactly where it came from and how sure the
 * parser was; the candidate's confirm, edit or reject is the last word; and an
 * edit never destroys the parsed original, which stays visible under the
 * correction. The degradation states are PRO-03's, shown rather than hidden:
 * a pending read says so, a format we cannot read says so and offers a
 * different file, and a failed read leaves the profile fully usable.
 */
export function CvSection() {
  const documents = useQuery({
    queryKey: ["documents"],
    queryFn: listDocuments,
  });
  const current = documents.data ? currentDocument(documents.data) : undefined;

  if (documents.isPending) {
    return (
      <LoadingSurface label="your CV">
        <SkeletonText width="50" />
        <SkeletonText />
        <SkeletonBlock />
      </LoadingSurface>
    );
  }

  if (documents.isError) {
    const failure = documents.error;
    return (
      <ErrorState
        what="Your CV could not be loaded"
        safe="Your CV and everything parsed from it are unaffected; only this view failed."
        reference={
          failure instanceof ApiError && failure.requestId
            ? failure.requestId
            : "none"
        }
        action={
          <Button type="button" onClick={() => void documents.refetch()}>
            Try again
          </Button>
        }
      />
    );
  }

  if (!current) {
    return (
      <EmptyState
        title="No CV yet"
        action={<UploadControl label="Upload your CV" />}
      >
        Upload your CV and we will parse roles, dates, skills and achievements
        from it - every one shown to you here, with where it came from, for you
        to confirm or correct.
      </EmptyState>
    );
  }

  switch (current.extraction_state) {
    case "pending":
      return (
        <DelayedState what="Your CV is uploaded and the reading is still running" />
      );
    case "unsupported":
      return (
        <UnassessableState
          what="We could not read this document"
          accepted="A plain text file works today; PDF and Word parsing are coming."
          action={<UploadControl label="Upload a different file" />}
        />
      );
    case "failed":
      return (
        <ErrorState
          what="Reading your CV failed"
          safe="Your CV itself is stored and your profile works without the parsing; you can also try a fresh upload."
          reference="extraction"
          action={<UploadControl label="Upload again" />}
        />
      );
    default:
      return <FactsList documentId={current.id} />;
  }
}

/**
 * The hidden-input upload control, labelled like the button it looks like.
 * The input is the accessible control; the styling is the design system's.
 */
function UploadControl({ label }: { label: string }) {
  const client = useQueryClient();
  const input = useRef<HTMLInputElement>(null);
  const upload = useMutation({
    mutationFn: (file: File) => uploadCv(file),
    onSettled: () => client.invalidateQueries({ queryKey: ["documents"] }),
  });

  const chosen = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (file) {
      upload.mutate(file);
    }
  };

  return (
    <span>
      <label
        className="inline-flex cursor-pointer items-center rounded-md bg-primary px-4 py-2 text-sm font-semibold text-primary-fg"
        htmlFor="cv-upload"
      >
        {upload.isPending ? "Uploading" : label}
        <input
          ref={input}
          id="cv-upload"
          type="file"
          accept=".pdf,.docx,.txt,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document,text/plain"
          className="sr-only"
          onChange={chosen}
          disabled={upload.isPending}
        />
      </label>
      {upload.isError ? (
        <span role="alert" className="mt-2 block text-sm text-danger">
          The upload did not finish; the failed attempt is recorded and
          uploading again simply takes the next version.
        </span>
      ) : null}
    </span>
  );
}

/** What extraction read, span-linked, with the candidate's word last. */
function FactsList({ documentId }: { documentId: string }) {
  const facts = useQuery({
    queryKey: ["facts", documentId],
    queryFn: () => listFacts(documentId),
  });

  if (facts.isPending) {
    return (
      <LoadingSurface label="what we parsed from your CV">
        <SkeletonText width="75" />
        <SkeletonText />
        <SkeletonText width="50" />
      </LoadingSurface>
    );
  }
  if (facts.isError) {
    const failure = facts.error;
    return (
      <ErrorState
        what="The parsed facts could not be loaded"
        safe="The facts themselves are unaffected; only this view failed."
        reference={
          failure instanceof ApiError && failure.requestId
            ? failure.requestId
            : "none"
        }
        action={
          <Button type="button" onClick={() => void facts.refetch()}>
            Try again
          </Button>
        }
      />
    );
  }

  const parsed = facts.data.filter((fact) => fact.kind !== "unparsed");
  const unparsed = facts.data.filter((fact) => fact.kind === "unparsed");

  return (
    <div className="space-y-6">
      <ul className="space-y-3">
        {parsed.map((fact) => (
          <FactRow key={fact.id} fact={fact} documentId={documentId} />
        ))}
      </ul>
      {unparsed.length > 0 ? (
        <section aria-labelledby="unparsed-heading">
          <h3 id="unparsed-heading" className="text-sm font-semibold">
            Text we could not parse
          </h3>
          <p className="mt-1 text-sm text-fg-2">
            Kept and shown rather than dropped, so the parsing never pretends to
            be more complete than it is.
          </p>
          <ul className="mt-3 space-y-3">
            {unparsed.map((fact) => (
              <FactRow key={fact.id} fact={fact} documentId={documentId} />
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  );
}

/** One fact: the reading, its provenance, and the candidate's controls. */
function FactRow({ fact, documentId }: { fact: Fact; documentId: string }) {
  const client = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");

  const review = useMutation({
    mutationFn: (body: Parameters<typeof reviewFact>[1]) =>
      reviewFact(fact.id, body),
    onSuccess: (updated) => {
      client.setQueryData<Fact[]>(["facts", documentId], (previous) =>
        (previous ?? []).map((each) =>
          each.id === updated.id ? updated : each,
        ),
      );
      setEditing(false);
    },
  });

  const text = factText(fact);
  const originalText = factText({ ...fact, corrected_value: undefined });
  const rejected = fact.status === "rejected";

  return (
    <li
      aria-label={text}
      className={`rounded-md border border-border bg-surface px-4 py-3 ${rejected ? "opacity-60" : ""}`}
    >
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <p
          className={`text-sm font-semibold ${rejected ? "line-through" : ""}`}
        >
          {text}
        </p>
        <span className="rounded-full border border-border px-2 py-0.5 text-2xs text-fg-2">
          {statusLabel(fact.status)}
        </span>
      </div>

      <p className="mt-1 text-2xs text-fg-3">
        {fact.kind.replace("_", " ")} · {Math.round(fact.confidence * 100)}%
        confident · {spanSentence(fact)}
      </p>
      <p className="sr-only">{Math.round(fact.confidence * 100)}% confident</p>

      {fact.status === "corrected" ? (
        <p className="mt-1 text-2xs text-fg-3">
          Parsed as {originalText}; your edit stands.
        </p>
      ) : null}

      {editing ? (
        <form
          className="mt-2 flex flex-wrap items-end gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            review.mutate({
              status: "corrected",
              corrected_value: correctionFor(fact, draft),
            });
          }}
        >
          <div className="flex-1">
            <label
              className="text-2xs text-fg-2"
              htmlFor={`fact-edit-${fact.id}`}
            >
              Your version
            </label>
            <input
              id={`fact-edit-${fact.id}`}
              className="mt-1 w-full rounded-md border border-border bg-surface px-2 py-1 text-sm"
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
            />
          </div>
          <Button type="submit" size="sm" busy={review.isPending}>
            Save
          </Button>
          <Button
            type="button"
            size="sm"
            variant="secondary"
            onClick={() => setEditing(false)}
          >
            Cancel
          </Button>
        </form>
      ) : (
        <div className="mt-2 flex flex-wrap gap-2">
          {rejected ? (
            <Button
              type="button"
              size="sm"
              variant="secondary"
              onClick={() => review.mutate({ status: "confirmed" })}
            >
              Restore
            </Button>
          ) : (
            <>
              {fact.status !== "confirmed" ? (
                <Button
                  type="button"
                  size="sm"
                  variant="secondary"
                  onClick={() => review.mutate({ status: "confirmed" })}
                >
                  Confirm
                </Button>
              ) : null}
              <Button
                type="button"
                size="sm"
                variant="secondary"
                onClick={() => {
                  setDraft(text);
                  setEditing(true);
                }}
              >
                Edit
              </Button>
              <Button
                type="button"
                size="sm"
                variant="secondary"
                onClick={() => review.mutate({ status: "rejected" })}
              >
                Reject
              </Button>
            </>
          )}
        </div>
      )}

      {review.isError ? (
        <p role="alert" className="mt-2 text-sm text-danger">
          That change did not save; the fact is unchanged. Try again.
        </p>
      ) : null}
    </li>
  );
}
