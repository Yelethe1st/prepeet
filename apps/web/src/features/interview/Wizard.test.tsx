import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QueryProvider } from "@/lib/api/QueryProvider";
import { ApiError } from "@/lib/api/client";

import { Wizard } from "./Wizard";
import * as api from "./api";
import type { Catalogue } from "./rules";

vi.mock("./api");

/**
 * The wizard, from the outside: steps addressable and restorable, entered
 * data preserved through refusals, focus moved to the first problem, and the
 * submission built from the catalogue's own vocabulary. The pure rules are
 * wizard.ts's suite; what is asserted here is the behaviour around them.
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
      description: "Competency questions.",
      minutes: [15, 25, 40],
    },
    {
      id: "shape_technical",
      name: "Technical deep-dive",
      description: "Verbal reasoning.",
      minutes: [25, 40],
    },
    {
      id: "shape_panel",
      name: "Panel simulation",
      description: "Rotating viewpoints.",
      minutes: [40, 60],
    },
  ],
  personas: [
    {
      id: "per_ama",
      name: "Ama",
      style: "Warm and structured",
      voice: "v",
      description: "Gentle.",
      best_for: "First sessions",
      shapes: [],
    },
    {
      id: "per_lena",
      name: "Lena",
      style: "Panel chair",
      voice: "v",
      description: "Formal.",
      best_for: "Panels",
      shapes: ["shape_panel"],
    },
  ],
};

const consent = {
  version: "1.0.0",
  title: "What we keep from this session",
  statements: [
    "This is a practice session. The recording, transcript and scores are visible only to you.",
    "Either way, you agree to recording once more on the device-check screen.",
  ],
  choices: {
    audio_and_transcript: {
      label: "The audio and the transcript",
      explanation: "Needed for replay and for delivery coaching.",
    },
    transcript_only: {
      label: "The transcript only",
      explanation: "Audio is discarded the moment the session ends.",
      forfeits: [
        "Replay of this session",
        "Delivery measurement for this session",
      ],
    },
  },
};

function renderWizard(initial?: {
  step?: number;
  selection?: Record<string, string>;
}) {
  vi.mocked(api.fetchCatalogue).mockResolvedValue(catalogue);
  vi.mocked(api.fetchPracticeConsent).mockResolvedValue(consent);
  return render(
    <Wizard
      initialStep={initial?.step}
      initialSelection={initial?.selection}
    />,
    {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryProvider>{children}</QueryProvider>
      ),
    },
  );
}

afterEach(() => {
  vi.mocked(api.fetchCatalogue).mockReset();
  vi.mocked(api.fetchPracticeConsent).mockReset();
  vi.mocked(api.createInterview).mockReset();
});

describe("the steps", () => {
  it("announces loading, then opens on the role step", async () => {
    renderWizard();

    expect(screen.getByRole("status")).toHaveTextContent(
      /loading the catalogue/i,
    );
    expect(
      await screen.findByRole("heading", { name: /^role$/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("radio", { name: /senior backend engineer/i }),
    ).toBeInTheDocument();
    // Nothing is hardcoded: the nursing role from the catalogue is offered too.
    expect(
      screen.getByRole("radio", { name: /registered nurse/i }),
    ).toBeInTheDocument();
  });

  it("refuses to advance without a choice, says why, and moves focus to the problem", async () => {
    renderWizard();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /continue/i }));

    const problem = screen.getByText(/choose a role to continue/i);
    expect(problem).toBeInTheDocument();
    expect(problem).toHaveFocus();
    expect(
      screen.getByRole("heading", { name: /^role$/i }),
    ).toBeInTheDocument();
  });

  it("walks forward, offering only what the previous choices allow", async () => {
    renderWizard();
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("radio", { name: /registered nurse/i }),
    );
    await user.click(screen.getByRole("button", { name: /continue/i }));

    // The nursing role offers only the panel shape.
    expect(
      await screen.findByRole("heading", { name: /interview shape/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("radio", { name: /panel simulation/i }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("radio", { name: /technical deep-dive/i }),
    ).not.toBeInTheDocument();
  });

  it("restores an addressed step with its selection intact", async () => {
    renderWizard({
      step: 3,
      selection: { role: "rl_rn", shape: "shape_panel" },
    });

    expect(
      await screen.findByRole("heading", { name: /interviewer/i }),
    ).toBeInTheDocument();
    // Lena runs panels; Ama is unrestricted. Both offered.
    expect(screen.getByRole("radio", { name: /lena/i })).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: /ama/i })).toBeInTheDocument();
    // Going back shows the earlier choice still made.
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /back/i }));
    expect(
      screen.getByRole("radio", { name: /panel simulation/i }),
    ).toBeChecked();
  });
});

describe("review and creation", () => {
  async function walkToReview(user: ReturnType<typeof userEvent.setup>) {
    await user.click(
      await screen.findByRole("radio", { name: /senior backend engineer/i }),
    );
    await user.click(screen.getByRole("button", { name: /continue/i }));
    await user.click(
      await screen.findByRole("radio", { name: /technical deep-dive/i }),
    );
    await user.click(screen.getByRole("button", { name: /continue/i }));
    await user.click(await screen.findByRole("radio", { name: /ama/i }));
    await user.click(screen.getByRole("button", { name: /continue/i }));
    await user.click(await screen.findByRole("radio", { name: /40 minutes/i }));
    await user.click(screen.getByRole("button", { name: /continue/i }));
    await screen.findByRole("heading", { name: /review/i });
  }

  it("submits the selection with its discipline, and reports the composing session", async () => {
    vi.mocked(api.createInterview).mockResolvedValue({
      id: "00000000-0000-7000-8000-0000000000e1",
      mode: "practice",
      state: "composing",
      config: {
        discipline: "software-engineering",
        role: "rl_swe",
        shape: "shape_technical",
        minutes: 40,
        persona: "per_ama",
      },
      recording_preference: "audio_and_transcript",
      consent_version: "1.0.0",
      created_at: "2026-08-25T10:00:00Z",
    });
    renderWizard();
    const user = userEvent.setup();

    await walkToReview(user);
    expect(screen.getByText(/senior backend engineer/i)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /start composing/i }));

    expect(api.createInterview).toHaveBeenCalledWith({
      mode: "practice",
      discipline: "software-engineering",
      role: "rl_swe",
      shape: "shape_technical",
      minutes: 40,
      persona: "per_ama",
      recording: {
        preference: "audio_and_transcript",
        consent_version: "1.0.0",
      },
    });
    expect(await screen.findByText(/being composed/i)).toBeInTheDocument();
  });

  it("shows the consent text beside the choice, and audio is the prototype's default", async () => {
    renderWizard();
    const user = userEvent.setup();

    await walkToReview(user);

    expect(
      screen.getByText(/what we keep from this session/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/visible only to you/i)).toBeInTheDocument();
    expect(
      screen.getByText(/once more on the device-check screen/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("radio", { name: /audio and the transcript/i }),
    ).toBeChecked();
  });

  it("choosing transcript-only names what it forfeits, visibly", async () => {
    vi.mocked(api.createInterview).mockResolvedValue({
      id: "00000000-0000-7000-8000-0000000000e1",
      mode: "practice",
      state: "composing",
      config: {
        discipline: "software-engineering",
        role: "rl_swe",
        shape: "shape_technical",
        minutes: 40,
        persona: "per_ama",
      },
      recording_preference: "transcript_only",
      consent_version: "1.0.0",
      created_at: "2026-08-26T10:00:00Z",
    });
    renderWizard();
    const user = userEvent.setup();

    await walkToReview(user);
    await user.click(screen.getByRole("radio", { name: /transcript only/i }));

    // The second criterion: the forfeit is named at the moment of choosing,
    // not implied - replay and delivery measurement, by name.
    expect(screen.getByText(/you are choosing to lose/i)).toBeInTheDocument();
    expect(screen.getByText(/replay of this session/i)).toBeInTheDocument();
    expect(screen.getByText(/delivery measurement/i)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /start composing/i }));
    expect(api.createInterview).toHaveBeenCalledWith(
      expect.objectContaining({
        recording: { preference: "transcript_only", consent_version: "1.0.0" },
      }),
    );
  });

  it("a stale consent refusal stays on review with the current text refetched", async () => {
    vi.mocked(api.createInterview).mockRejectedValue(
      new ApiError({
        status: 400,
        message: "Some of the details were not accepted.",
        fieldErrors: {
          "recording.consent_version":
            "The consent text has changed since it was shown. Review the current text and choose again.",
        },
      }),
    );
    renderWizard();
    const user = userEvent.setup();

    await walkToReview(user);
    vi.mocked(api.fetchPracticeConsent).mockClear();
    await user.click(screen.getByRole("button", { name: /start composing/i }));

    expect(
      await screen.findByText(/consent text has changed/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: /review/i }),
    ).toBeInTheDocument();
    await waitFor(() => expect(api.fetchPracticeConsent).toHaveBeenCalled());
  });

  it("returns to the refused field's own step with everything preserved and the error focused", async () => {
    vi.mocked(api.createInterview).mockRejectedValue(
      new ApiError({
        status: 400,
        message: "Some of the details were not accepted.",
        fieldErrors: {
          persona: "That interviewer does not run the chosen interview shape.",
        },
      }),
    );
    renderWizard();
    const user = userEvent.setup();

    await walkToReview(user);
    await user.click(screen.getByRole("button", { name: /start composing/i }));

    // Back on the interviewer step, refusal focused, choices intact.
    expect(
      await screen.findByRole("heading", { name: /interviewer/i }),
    ).toBeInTheDocument();
    const problem = screen.getByText(
      /does not run the chosen interview shape/i,
    );
    await waitFor(() => expect(problem).toHaveFocus());
    expect(screen.getByRole("radio", { name: /ama/i })).toBeChecked();
    await user.click(screen.getByRole("button", { name: /back/i }));
    expect(
      screen.getByRole("radio", { name: /technical deep-dive/i }),
    ).toBeChecked();
  });
});
