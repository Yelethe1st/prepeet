import { describe, expect, it } from "vitest";

import {
  STEPS,
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
 * The wizard's rules, pure: which options each step offers given what is
 * already chosen, what blocks advancing, and how a selection survives a
 * change that invalidates part of it. The component renders these; it must
 * not reinterpret them.
 */

const catalogue: Catalogue = {
  disciplines: [
    { id: "software-engineering", name: "Software engineering" },
    { id: "nursing", name: "Nursing" },
  ],
  roles: [
    {
      id: "rl_swe",
      discipline: "software-engineering",
      title: "Senior Backend Engineer",
      organisation: "Product company",
      competencies: ["Systems design"],
      shapes: ["shape_behavioural", "shape_technical"],
    },
    {
      id: "rl_rn",
      discipline: "nursing",
      title: "Registered Nurse",
      organisation: "Health system",
      competencies: ["Clinical reasoning"],
      shapes: ["shape_panel"],
    },
  ],
  shapes: [
    {
      id: "shape_behavioural",
      name: "Behavioural",
      description: "x",
      minutes: [15, 25, 40],
    },
    {
      id: "shape_technical",
      name: "Technical deep-dive",
      description: "x",
      minutes: [25, 40],
    },
    {
      id: "shape_panel",
      name: "Panel simulation",
      description: "x",
      minutes: [40, 60],
    },
  ],
  personas: [
    {
      id: "per_ama",
      name: "Ama",
      style: "Warm",
      voice: "v",
      description: "x",
      best_for: "b",
      shapes: [],
    },
    {
      id: "per_lena",
      name: "Lena",
      style: "Panel chair",
      voice: "v",
      description: "x",
      best_for: "b",
      shapes: ["shape_panel"],
    },
  ],
};

const chosen: Selection = {
  role: "rl_swe",
  shape: "shape_technical",
  persona: "per_ama",
  minutes: 40,
};

describe("the options each step offers", () => {
  it("offers only the chosen role's shapes", () => {
    expect(shapesFor(catalogue, "rl_swe").map((shape) => shape.id)).toEqual([
      "shape_behavioural",
      "shape_technical",
    ]);
    expect(shapesFor(catalogue, "rl_rn").map((shape) => shape.id)).toEqual([
      "shape_panel",
    ]);
  });

  it("offers only personas that run the chosen shape, unrestricted meaning all", () => {
    expect(
      personasFor(catalogue, "shape_technical").map((persona) => persona.id),
    ).toEqual(["per_ama"]);
    expect(
      personasFor(catalogue, "shape_panel").map((persona) => persona.id),
    ).toEqual(["per_ama", "per_lena"]);
  });

  it("offers only the chosen shape's lengths", () => {
    expect(minutesFor(catalogue, "shape_technical")).toEqual([25, 40]);
  });
});

describe("what blocks a step", () => {
  it("names the missing choice, step by step", () => {
    expect(stepProblem(catalogue, {}, 1)).toMatch(/choose a role/i);
    expect(stepProblem(catalogue, { role: "rl_swe" }, 1)).toBeNull();
    expect(stepProblem(catalogue, { role: "rl_swe" }, 2)).toMatch(
      /choose an interview shape/i,
    );
    expect(stepProblem(catalogue, chosen, 2)).toBeNull();
    expect(
      stepProblem(catalogue, { role: "rl_swe", shape: "shape_technical" }, 3),
    ).toMatch(/choose an interviewer/i);
    expect(
      stepProblem(catalogue, { ...chosen, minutes: undefined }, 4),
    ).toMatch(/choose a length/i);
    expect(stepProblem(catalogue, chosen, 4)).toBeNull();
  });

  it("refuses a combination the catalogue does not offer, not only an absence", () => {
    expect(
      stepProblem(catalogue, { role: "rl_rn", shape: "shape_technical" }, 2),
    ).toMatch(/does not offer/i);
    expect(
      stepProblem(
        catalogue,
        {
          role: "rl_rn",
          shape: "shape_panel",
          persona: "per_lena",
          minutes: 15,
        },
        4,
      ),
    ).toMatch(/does not run/i);
  });
});

describe("a change that invalidates later choices", () => {
  it("trims exactly the choices the new selection no longer offers", () => {
    // Switching to the nursing role: technical is gone, so shape and its
    // dependants clear - but nothing the person chose that still fits is
    // touched.
    const trimmed = trimSelection(catalogue, { ...chosen, role: "rl_rn" });

    expect(trimmed.role).toBe("rl_rn");
    expect(trimmed.shape).toBeUndefined();
    expect(trimmed.persona).toBeUndefined();
    expect(trimmed.minutes).toBeUndefined();
  });

  it("keeps everything that still fits", () => {
    const trimmed = trimSelection(catalogue, {
      ...chosen,
      shape: "shape_behavioural",
    });

    expect(trimmed.persona).toBe("per_ama");
    expect(trimmed.minutes).toBe(40);
  });
});

describe("addressing", () => {
  it("maps each server field to the step that owns it", () => {
    expect(stepFor("role")).toBe(1);
    expect(stepFor("discipline")).toBe(1);
    expect(stepFor("shape")).toBe(2);
    expect(stepFor("persona")).toBe(3);
    expect(stepFor("minutes")).toBe(4);
  });

  it("has five steps, review last", () => {
    expect(STEPS).toHaveLength(5);
    expect(STEPS[4]).toMatch(/review/i);
  });
});
