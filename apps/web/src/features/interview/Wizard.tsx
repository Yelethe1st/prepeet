"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";

import { ApiError } from "@/lib/api/client";
import { Button } from "@/shared/components";
import {
  ErrorState,
  LoadingSurface,
  SkeletonBlock,
  SkeletonText,
} from "@/shared/states";

import { createInterview, fetchCatalogue, fetchPracticeConsent } from "./api";
import type { CreateInterviewRequest, PracticeConsent } from "./api";
import {
  STEPS,
  disciplineOf,
  minutesFor,
  personasFor,
  shapesFor,
  stepFor,
  stepProblem,
  trimSelection,
  type Catalogue,
  type Selection,
} from "./rules";

/**
 * The practice configuration wizard - CAT-04, from the prototype's
 * candidate-start-interview screen.
 *
 * Every option comes from the catalogue endpoints; nothing here knows a
 * discipline's name. Each step is addressable (the URL carries the step and
 * the selection, so a copied link restores both), validated independently
 * (advancing is refused with the problem named and focused), and nothing
 * entered is ever discarded - a refusal, local or server, returns to the
 * offending step with every other choice intact. The wizard only speaks
 * practice; the screening refusal is the server's, and stronger there.
 */
export function Wizard({
  initialStep,
  initialSelection,
}: {
  initialStep?: number;
  initialSelection?: Record<string, string>;
}) {
  const catalogue = useQuery({
    queryKey: ["catalogue"],
    queryFn: fetchCatalogue,
  });
  const consent = useQuery({
    queryKey: ["practice-consent"],
    queryFn: fetchPracticeConsent,
  });

  const [step, setStep] = useState(boundStep(initialStep));
  const [selection, setSelection] = useState<Selection>(() => ({
    role: initialSelection?.role,
    shape: initialSelection?.shape,
    persona: initialSelection?.persona,
    minutes: initialSelection?.minutes
      ? Number(initialSelection.minutes)
      : undefined,
  }));
  const [problem, setProblem] = useState<string | null>(null);
  const problemRef = useRef<HTMLParagraphElement>(null);
  // The recording choice, defaulting to the prototype's: audio and
  // transcript. The consent text beside it is what makes the default a
  // choice rather than a silence.
  const [recording, setRecording] = useState<
    "audio_and_transcript" | "transcript_only"
  >("audio_and_transcript");

  // The URL mirrors the state so the step is addressable and a reload or a
  // shared link restores everything entered. Replace rather than push: the
  // browser's back button leaves the wizard, it does not unpick choices.
  useEffect(() => {
    const parameters = new URLSearchParams();
    parameters.set("step", String(step));
    if (selection.role) parameters.set("role", selection.role);
    if (selection.shape) parameters.set("shape", selection.shape);
    if (selection.persona) parameters.set("persona", selection.persona);
    if (selection.minutes) parameters.set("minutes", String(selection.minutes));
    window.history.replaceState(null, "", `?${parameters.toString()}`);
  }, [step, selection]);

  // A freshly shown problem takes focus, so keyboard and screen-reader
  // users land on what blocked them rather than hunting for it.
  useEffect(() => {
    if (problem) {
      problemRef.current?.focus();
    }
  }, [problem]);

  const create = useMutation({
    mutationFn: (request: CreateInterviewRequest) => createInterview(request),
    onError: (error: unknown) => {
      if (error instanceof ApiError) {
        const [field, message] = Object.entries(error.fieldErrors)[0] ?? [];
        if (field && message) {
          // The server refused a combination: return to the field's own
          // step with the refusal focused and everything else preserved. A
          // stale consent version additionally refetches the text, so what
          // is shown is what the next attempt agrees to.
          if (field === "recording.consent_version") {
            void consent.refetch();
          }
          setStep(stepFor(field));
          setProblem(message);
          return;
        }
      }
      setProblem(
        "The interview could not be created. Nothing you chose is lost; try again.",
      );
    },
  });

  if (catalogue.isPending || consent.isPending) {
    return (
      <LoadingSurface label="the catalogue">
        <SkeletonText width="50" />
        <SkeletonBlock />
        <SkeletonBlock />
      </LoadingSurface>
    );
  }
  if (catalogue.isError || consent.isError) {
    const failure = catalogue.isError ? catalogue.error : consent.error;
    return (
      <ErrorState
        what="The catalogue could not be loaded"
        safe="Nothing about your account is affected; the interview options just did not arrive."
        reference={
          failure instanceof ApiError && failure.requestId
            ? failure.requestId
            : "none"
        }
        action={
          <Button type="button" onClick={() => void catalogue.refetch()}>
            Try again
          </Button>
        }
      />
    );
  }
  const data = catalogue.data;
  const consentDocument = consent.data;

  if (create.isSuccess) {
    return (
      <section aria-labelledby="composed-heading" className="max-w-[560px]">
        <h2 id="composed-heading" className="text-lg font-semibold">
          Your interview is being composed
        </h2>
        <p className="mt-2 text-sm text-fg-2">
          We are pinning the exact plan, questions and interviewer configuration
          for this session. Head to the prepare screen for the brief, the device
          checks and consent - nothing is recorded until you press start there.
        </p>
        <a
          className="mt-4 inline-flex items-center rounded-md bg-primary px-4 py-2 text-sm font-semibold text-primary-fg"
          href={`/session/${create.data.id}/prepare`}
        >
          Go to the prepare screen
        </a>
        <p className="mt-2 font-mono text-2xs text-fg-3">
          Session {create.data.id}
        </p>
      </section>
    );
  }

  const choose = (change: Partial<Selection>) => {
    setProblem(null);
    setSelection((previous) => trimSelection(data, { ...previous, ...change }));
  };

  const advance = () => {
    const blocked = stepProblem(data, selection, step);
    if (blocked) {
      setProblem(blocked);
      return;
    }
    setProblem(null);
    setStep((previous) => Math.min(previous + 1, STEPS.length));
  };

  const submit = () => {
    setProblem(null);
    create.mutate({
      mode: "practice",
      discipline: disciplineOf(data, selection.role),
      role: selection.role ?? "",
      shape: selection.shape ?? "",
      minutes: selection.minutes ?? 0,
      persona: selection.persona ?? "",
      recording: {
        preference: recording,
        consent_version: consentDocument.version,
      },
    });
  };

  return (
    <div className="max-w-[720px]">
      <nav aria-label="Wizard steps">
        <ol className="flex flex-wrap gap-2 text-2xs text-fg-3">
          {STEPS.map((label, index) => (
            <li
              key={label}
              aria-current={index + 1 === step ? "step" : undefined}
              className={index + 1 === step ? "font-semibold text-fg" : ""}
            >
              {index + 1}. {label}
            </li>
          ))}
        </ol>
      </nav>

      <h2 className="mt-4 text-lg font-semibold">{STEPS[step - 1]}</h2>

      <div className="mt-4">
        {step === 1 ? (
          <RoleStep catalogue={data} selection={selection} choose={choose} />
        ) : null}
        {step === 2 ? (
          <ShapeStep catalogue={data} selection={selection} choose={choose} />
        ) : null}
        {step === 3 ? (
          <PersonaStep catalogue={data} selection={selection} choose={choose} />
        ) : null}
        {step === 4 ? (
          <LengthStep catalogue={data} selection={selection} choose={choose} />
        ) : null}
        {step === 5 ? (
          <ReviewStep
            catalogue={data}
            selection={selection}
            consent={consentDocument}
            recording={recording}
            onRecording={setRecording}
          />
        ) : null}
      </div>

      {problem ? (
        <p
          ref={problemRef}
          tabIndex={-1}
          role="alert"
          className="mt-4 text-sm font-semibold text-danger"
        >
          {problem}
        </p>
      ) : null}

      <div className="mt-6 flex gap-2">
        {step > 1 ? (
          <Button
            type="button"
            variant="secondary"
            onClick={() => {
              setProblem(null);
              setStep((previous) => Math.max(previous - 1, 1));
            }}
          >
            Back
          </Button>
        ) : null}
        {step < STEPS.length ? (
          <Button type="button" onClick={advance}>
            Continue
          </Button>
        ) : (
          <Button type="button" onClick={submit} busy={create.isPending}>
            Start composing
          </Button>
        )}
      </div>
    </div>
  );
}

function boundStep(step: number | undefined): number {
  if (!step || step < 1) {
    return 1;
  }
  return Math.min(step, STEPS.length);
}

/** One radio card group, labelled by the step. */
function Options({
  legend,
  children,
}: {
  legend: string;
  children: ReactNode;
}) {
  return (
    <fieldset>
      <legend className="sr-only">{legend}</legend>
      <div className="space-y-2">{children}</div>
    </fieldset>
  );
}

function OptionCard({
  name,
  value,
  checked,
  onChoose,
  title,
  children,
}: {
  name: string;
  value: string;
  checked: boolean;
  onChoose: () => void;
  title: string;
  children?: ReactNode;
}) {
  return (
    <label className="flex cursor-pointer gap-3 rounded-md border border-border bg-surface px-4 py-3 has-[:checked]:border-primary">
      <input
        type="radio"
        name={name}
        value={value}
        checked={checked}
        onChange={onChoose}
        className="mt-1"
      />
      <span>
        <span className="block text-sm font-semibold">{title}</span>
        {children ? (
          <span className="block text-sm text-fg-2">{children}</span>
        ) : null}
      </span>
    </label>
  );
}

function RoleStep({
  catalogue,
  selection,
  choose,
}: {
  catalogue: Catalogue;
  selection: Selection;
  choose: (change: Partial<Selection>) => void;
}) {
  return (
    <Options legend="Role">
      {catalogue.disciplines.map((discipline) => {
        const roles = catalogue.roles.filter(
          (role) => role.discipline === discipline.id,
        );
        if (roles.length === 0) {
          return null;
        }
        return (
          <div key={discipline.id}>
            <h3 className="mt-3 mb-2 text-2xs font-semibold tracking-wide text-fg-3 uppercase">
              {discipline.name}
            </h3>
            {roles.map((role) => (
              <OptionCard
                key={role.id}
                name="role"
                value={role.id}
                checked={selection.role === role.id}
                onChoose={() => choose({ role: role.id })}
                title={role.title}
              >
                {role.organisation} · {role.competencies.join(", ")}
              </OptionCard>
            ))}
          </div>
        );
      })}
    </Options>
  );
}

function ShapeStep({
  catalogue,
  selection,
  choose,
}: {
  catalogue: Catalogue;
  selection: Selection;
  choose: (change: Partial<Selection>) => void;
}) {
  return (
    <Options legend="Interview shape">
      {shapesFor(catalogue, selection.role).map((shape) => (
        <OptionCard
          key={shape.id}
          name="shape"
          value={shape.id}
          checked={selection.shape === shape.id}
          onChoose={() => choose({ shape: shape.id })}
          title={shape.name}
        >
          {shape.description}
        </OptionCard>
      ))}
    </Options>
  );
}

function PersonaStep({
  catalogue,
  selection,
  choose,
}: {
  catalogue: Catalogue;
  selection: Selection;
  choose: (change: Partial<Selection>) => void;
}) {
  return (
    <div>
      <Options legend="Interviewer">
        {personasFor(catalogue, selection.shape).map((persona) => (
          <OptionCard
            key={persona.id}
            name="persona"
            value={persona.id}
            checked={selection.persona === persona.id}
            onChoose={() => choose({ persona: persona.id })}
            title={`${persona.name} · ${persona.style}`}
          >
            {persona.description} Best for {persona.best_for}.
          </OptionCard>
        ))}
      </Options>
      <p className="mt-3 text-sm text-fg-2">
        A persona is an interview style - pacing, follow-up pressure, silence.
        It is never a judgement about you, and the same rubric and evidence
        standard apply whichever you pick.
      </p>
    </div>
  );
}

function LengthStep({
  catalogue,
  selection,
  choose,
}: {
  catalogue: Catalogue;
  selection: Selection;
  choose: (change: Partial<Selection>) => void;
}) {
  return (
    <Options legend="Length">
      {minutesFor(catalogue, selection.shape).map((minutes) => (
        <OptionCard
          key={minutes}
          name="minutes"
          value={String(minutes)}
          checked={selection.minutes === minutes}
          onChoose={() => choose({ minutes })}
          title={`${minutes} minutes`}
        />
      ))}
    </Options>
  );
}

function ReviewStep({
  catalogue,
  selection,
  consent,
  recording,
  onRecording,
}: {
  catalogue: Catalogue;
  selection: Selection;
  consent: PracticeConsent;
  recording: "audio_and_transcript" | "transcript_only";
  onRecording: (preference: "audio_and_transcript" | "transcript_only") => void;
}) {
  const role = catalogue.roles.find((each) => each.id === selection.role);
  const shape = catalogue.shapes.find((each) => each.id === selection.shape);
  const persona = catalogue.personas.find(
    (each) => each.id === selection.persona,
  );
  return (
    <div className="space-y-6">
      <dl className="space-y-2 text-sm">
        <div className="flex justify-between gap-4">
          <dt className="text-fg-2">Role</dt>
          <dd className="text-right font-semibold">{role?.title}</dd>
        </div>
        <div className="flex justify-between gap-4">
          <dt className="text-fg-2">Shape</dt>
          <dd className="text-right font-semibold">{shape?.name}</dd>
        </div>
        <div className="flex justify-between gap-4">
          <dt className="text-fg-2">Interviewer</dt>
          <dd className="text-right font-semibold">{persona?.name}</dd>
        </div>
        <div className="flex justify-between gap-4">
          <dt className="text-fg-2">Length</dt>
          <dd className="text-right font-semibold">
            {selection.minutes} minutes
          </dd>
        </div>
      </dl>

      <div className="rounded-md border border-info-border bg-info-soft px-4 py-3 text-sm text-info-fg">
        {consent.statements.map((statement) => (
          <p key={statement} className="mt-1 first:mt-0">
            {statement}
          </p>
        ))}
      </div>

      <fieldset className="rounded-md border border-border bg-surface px-4 py-3">
        <legend className="px-1 text-sm font-semibold">{consent.title}</legend>
        <div className="space-y-2">
          <label className="flex cursor-pointer gap-3">
            <input
              type="radio"
              name="recording"
              value="audio_and_transcript"
              checked={recording === "audio_and_transcript"}
              onChange={() => onRecording("audio_and_transcript")}
              className="mt-1"
            />
            <span>
              <span className="block text-sm font-semibold">
                {consent.choices.audio_and_transcript.label}
              </span>
              <span className="block text-sm text-fg-2">
                {consent.choices.audio_and_transcript.explanation}
              </span>
            </span>
          </label>
          <label className="flex cursor-pointer gap-3">
            <input
              type="radio"
              name="recording"
              value="transcript_only"
              checked={recording === "transcript_only"}
              onChange={() => onRecording("transcript_only")}
              className="mt-1"
            />
            <span>
              <span className="block text-sm font-semibold">
                {consent.choices.transcript_only.label}
              </span>
              <span className="block text-sm text-fg-2">
                {consent.choices.transcript_only.explanation}
              </span>
            </span>
          </label>
          {recording === "transcript_only" &&
          consent.choices.transcript_only.forfeits ? (
            <div
              role="status"
              className="rounded-md border border-warning-border bg-warning-soft px-3 py-2 text-sm text-warning-fg"
            >
              <p className="font-semibold">
                You are choosing to lose, for this session:
              </p>
              <ul className="mt-1 list-disc pl-5">
                {consent.choices.transcript_only.forfeits.map((forfeit) => (
                  <li key={forfeit}>{forfeit}</li>
                ))}
              </ul>
            </div>
          ) : null}
        </div>
      </fieldset>
    </div>
  );
}
