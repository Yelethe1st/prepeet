import type { components } from "@contracts";

/**
 * The wizard's pure rules: what each step offers given what is chosen, what
 * blocks advancing, and how a selection survives a change that invalidates
 * part of it.
 *
 * The rules mirror the catalogue's combination data - a role's shapes, a
 * shape's lengths, a persona's shapes - and they are a courtesy: the server
 * validates the same selection against the same catalogue at creation, and
 * that refusal, not this filtering, is the rule.
 */

export type Discipline = components["schemas"]["Discipline"];
export type CatalogRole = components["schemas"]["CatalogRole"];
export type InterviewShape = components["schemas"]["InterviewShape"];
export type Persona = components["schemas"]["Persona"];

/** The four collections, fetched together. */
export interface Catalogue {
  disciplines: Discipline[];
  roles: CatalogRole[];
  shapes: InterviewShape[];
  personas: Persona[];
}

/** What the person has chosen so far. Partial until review. */
export interface Selection {
  role?: string;
  shape?: string;
  persona?: string;
  minutes?: number;
}

/** The steps, in order. The index in this list plus one is the step number. */
export const STEPS = [
  "Role",
  "Interview shape",
  "Interviewer",
  "Length",
  "Review and start",
] as const;

/** The shapes the chosen role offers, in catalogue order. */
export function shapesFor(
  catalogue: Catalogue,
  roleID: string | undefined,
): InterviewShape[] {
  const role = catalogue.roles.find((each) => each.id === roleID);
  if (!role) {
    return [];
  }
  return catalogue.shapes.filter((shape) => role.shapes.includes(shape.id));
}

/** The personas that run the chosen shape; an empty shapes list means all. */
export function personasFor(
  catalogue: Catalogue,
  shapeID: string | undefined,
): Persona[] {
  if (!shapeID) {
    return [];
  }
  return catalogue.personas.filter(
    (persona) =>
      persona.shapes.length === 0 || persona.shapes.includes(shapeID),
  );
}

/** The lengths the chosen shape runs at. */
export function minutesFor(
  catalogue: Catalogue,
  shapeID: string | undefined,
): number[] {
  return catalogue.shapes.find((shape) => shape.id === shapeID)?.minutes ?? [];
}

/** The discipline the chosen role belongs to, for the request body. */
export function disciplineOf(
  catalogue: Catalogue,
  roleID: string | undefined,
): string {
  return catalogue.roles.find((each) => each.id === roleID)?.discipline ?? "";
}

/**
 * What blocks one step, or null. The message is what the step's error line
 * shows and what focus moves to, so it names the choice rather than a code.
 */
export function stepProblem(
  catalogue: Catalogue,
  selection: Selection,
  step: number,
): string | null {
  switch (step) {
    case 1:
      return selection.role ? null : "Choose a role to continue.";
    case 2: {
      if (!selection.shape) {
        return "Choose an interview shape to continue.";
      }
      const offered = shapesFor(catalogue, selection.role).some(
        (shape) => shape.id === selection.shape,
      );
      return offered
        ? null
        : "The chosen role does not offer that interview shape.";
    }
    case 3: {
      if (!selection.persona) {
        return "Choose an interviewer to continue.";
      }
      const offered = personasFor(catalogue, selection.shape).some(
        (persona) => persona.id === selection.persona,
      );
      return offered
        ? null
        : "That interviewer does not run the chosen interview shape.";
    }
    case 4: {
      if (!selection.minutes) {
        return "Choose a length to continue.";
      }
      const offered = minutesFor(catalogue, selection.shape).includes(
        selection.minutes,
      );
      return offered
        ? null
        : "That interview shape does not run at that length.";
    }
    default:
      return null;
  }
}

/**
 * Drops exactly the choices the selection no longer offers, keeping
 * everything that still fits: changing your role should not cost you a
 * persona that works for the new shape too.
 */
export function trimSelection(
  catalogue: Catalogue,
  selection: Selection,
): Selection {
  const trimmed: Selection = { ...selection };
  if (
    trimmed.shape &&
    !shapesFor(catalogue, trimmed.role).some((s) => s.id === trimmed.shape)
  ) {
    delete trimmed.shape;
  }
  if (
    trimmed.persona &&
    !personasFor(catalogue, trimmed.shape).some((p) => p.id === trimmed.persona)
  ) {
    delete trimmed.persona;
  }
  if (
    trimmed.minutes &&
    !minutesFor(catalogue, trimmed.shape).includes(trimmed.minutes)
  ) {
    delete trimmed.minutes;
  }
  return trimmed;
}

/** The step that owns a server-refused field, for moving focus there. */
export function stepFor(field: string): number {
  switch (field) {
    case "discipline":
    case "role":
      return 1;
    case "shape":
      return 2;
    case "persona":
      return 3;
    case "minutes":
      return 4;
    default:
      return 5;
  }
}
