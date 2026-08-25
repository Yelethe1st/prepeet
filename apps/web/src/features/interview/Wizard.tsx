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

import { createInterview, fetchCatalogue } from "./api";
import type { CreateInterviewRequest } from "./api";
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
          // step with the refusal focused and everything else preserved.
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

  if (catalogue.isPending) {
    return (
      <LoadingSurface label="the catalogue">
        <SkeletonText width="50" />
        <SkeletonBlock />
        <SkeletonBlock />
      </LoadingSurface>
    );
  }
  if (catalogue.isError) {
    const failure = catalogue.error;
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

  if (create.isSuccess) {
    return (
      <section aria-labelledby="composed-heading" className="max-w-[560px]">
        <h2 id="composed-heading" className="text-lg font-semibold">
          Your interview is being composed
        </h2>
        <p className="mt-2 text-sm text-fg-2">
          We are pinning the exact plan, questions and interviewer configuration
          for this session. The prepare screen - device checks and consent -
          opens from here once it is built; until then your session is safe and
          visible to nobody but you.
        </p>
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
          <ReviewStep catalogue={data} selection={selection} />
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
}: {
  catalogue: Catalogue;
  selection: Selection;
}) {
  const role = catalogue.roles.find((each) => each.id === selection.role);
  const shape = catalogue.shapes.find((each) => each.id === selection.shape);
  const persona = catalogue.personas.find(
    (each) => each.id === selection.persona,
  );
  return (
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
  );
}
