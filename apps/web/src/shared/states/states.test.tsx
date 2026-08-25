import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { axe } from "vitest-axe";

import {
  ConnectionState,
  DegradedState,
  DelayedState,
  EmptyState,
  ErrorState,
  ExpiredState,
  ForbiddenState,
  InsufficientEvidenceState,
  PartialState,
  UnassessableState,
} from "./index";

/**
 * The cross-journey state contract, as components.
 *
 * user-journeys.md names eleven states every data surface must support, and
 * WEB-04's risk is each screen improvising its own: eleven dialects of
 * "something is off" that people have to relearn per page. These tests pin
 * the two halves of the contract: the required content is genuinely required
 * (rendered, not just accepted), and each state announces itself with the
 * urgency it actually has.
 */

describe("ErrorState", () => {
  const failed = () => (
    <ErrorState
      what="Your sessions could not be loaded"
      safe="Your sessions and evaluations are unaffected; only this view failed."
      reference="err_7Kq2XA"
      action={<button type="button">Try again</button>}
    />
  );

  /**
   * The content rule is the ticket's third box, verbatim: what failed, what is
   * still safe, the next action, and a reference identifier. Every field is a
   * required prop, so the rule holds at the type level; this asserts the props
   * actually reach the page rather than being swallowed.
   */
  it("shows what failed, what is safe, the action and the reference", () => {
    render(failed());

    expect(
      screen.getByRole("heading", { name: /could not be loaded/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/unaffected/i)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Try again" }),
    ).toBeInTheDocument();
    expect(screen.getByText("err_7Kq2XA")).toBeInTheDocument();
  });

  it("interrupts as an alert, because a failure is the one state that must", () => {
    render(failed());

    expect(screen.getByRole("alert")).toBeInTheDocument();
  });

  it("has no axe violations", async () => {
    const { container } = render(failed());

    expect(await axe(container)).toHaveNoViolations();
  });
});

describe("EmptyState", () => {
  /**
   * The prototype's empty pattern ("Nothing to plot yet") explains what the
   * surface will show and offers the act that creates the first item. Absence
   * with no way forward is a dead end wearing a friendly face.
   */
  it("names the surface, explains what will appear, and offers the creating action", () => {
    render(
      <EmptyState
        title="Nothing to plot yet"
        action={<a href="/practice">Start your first interview</a>}
      >
        Progression needs at least one finished session before it can show you
        anything.
      </EmptyState>,
    );

    expect(
      screen.getByRole("heading", { name: "Nothing to plot yet" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/at least one finished session/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /start your first interview/i }),
    ).toBeInTheDocument();
  });

  it("does not announce absence as a problem", () => {
    render(
      <EmptyState
        title="No documents yet"
        action={<button type="button">Upload a CV</button>}
      >
        Your CV will appear here once uploaded.
      </EmptyState>,
    );

    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

describe("PartialState", () => {
  /**
   * Partial is content plus honesty, not an error page: what loaded renders,
   * and the notice names exactly what is missing with a way to retry it. The
   * failure mode this exists against is the silent two-thirds of a page that
   * reads as the whole.
   */
  it("renders the loaded content beside a notice naming what is missing", () => {
    render(
      <PartialState
        missing="Your recent evaluations did not load"
        action={<button type="button">Retry that section</button>}
      >
        <p>Session history</p>
      </PartialState>,
    );

    expect(screen.getByText("Session history")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent(
      /recent evaluations did not load/i,
    );
    expect(
      screen.getByRole("button", { name: /retry that section/i }),
    ).toBeInTheDocument();
  });
});

describe("ForbiddenState", () => {
  /**
   * In-surface forbidden says what is closed and who can open it, and offers
   * no button pretending otherwise: the server refused, and a retry that will
   * refuse identically is not an action, it is a lie with a spinner.
   */
  it("names what is closed and who can grant it", () => {
    render(
      <ForbiddenState
        what="Evaluation details"
        grantedBy="a workspace admin"
      />,
    );

    expect(
      screen.getByRole("heading", { name: /evaluation details/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/workspace admin/i)).toBeInTheDocument();
  });
});

describe("ExpiredState", () => {
  it("says what expired and offers the renewal", () => {
    render(
      <ExpiredState
        what="This invitation has expired"
        action={<button type="button">Request a new invitation</button>}
      />,
    );

    expect(
      screen.getByRole("heading", { name: /invitation has expired/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /request a new invitation/i }),
    ).toBeInTheDocument();
  });
});

describe("DelayedState", () => {
  /**
   * The journey spec's promise: no timed interaction silently discards work.
   * Delayed therefore always says the work is safe and leaving is fine; a
   * spinner alone teaches people to sit and guard a page.
   */
  it("says what is delayed, that nothing is lost, and that leaving is safe", () => {
    render(<DelayedState what="Your evaluation is taking longer than usual" />);

    expect(
      screen.getByRole("heading", { name: /taking longer/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/nothing.*lost/i)).toBeInTheDocument();
    expect(screen.getByText(/safe to leave/i)).toBeInTheDocument();
    expect(screen.getByRole("status")).toBeInTheDocument();
  });
});

describe("InsufficientEvidenceState", () => {
  /**
   * The prototype is explicit that an empty track must not read as "scored
   * zero". Insufficient evidence is a neutral fact with a remedy, and the
   * component must never render a zero or claim failure.
   */
  it("presents the gap neutrally with the remedy", () => {
    render(
      <InsufficientEvidenceState
        what="Prioritisation under pressure"
        remedy="A case-format session would surface three or four answers."
      />,
    );

    expect(screen.getByText(/insufficient evidence/i)).toBeInTheDocument();
    expect(screen.getByText(/case-format session/i)).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.queryByText(/^0$/)).not.toBeInTheDocument();
    expect(screen.queryByText(/fail/i)).not.toBeInTheDocument();
  });
});

describe("UnassessableState", () => {
  it("says what could not be read, what would be readable, and how to provide it", () => {
    render(
      <UnassessableState
        what="We could not read this document"
        accepted="A PDF, Word document or plain text file works."
        action={<button type="button">Upload a different file</button>}
      />,
    );

    expect(
      screen.getByRole("heading", { name: /could not read/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/plain text file/i)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /different file/i }),
    ).toBeInTheDocument();
  });
});

describe("ConnectionState", () => {
  /**
   * Reconnecting and recovered are one component because they are one story,
   * and a live screen swaps the phase rather than the markup. Both announce
   * politely: an assertive interruption mid-interview is worse than the blip.
   */
  it("announces reconnecting as a status", () => {
    render(<ConnectionState phase="reconnecting" what="the interview" />);

    expect(screen.getByRole("status")).toHaveTextContent(/reconnecting/i);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("announces recovery, and that the connection held work safe", () => {
    render(<ConnectionState phase="recovered" what="the interview" />);

    expect(screen.getByRole("status")).toHaveTextContent(/reconnected/i);
    expect(screen.getByRole("status")).toHaveTextContent(/nothing.*lost/i);
  });
});

describe("DegradedState", () => {
  /**
   * A provider incident names what is degraded and what still works, so a
   * person can decide whether to continue rather than discovering the edge
   * mid-task.
   */
  it("names what is degraded and what still works", () => {
    render(
      <DegradedState
        what="Live interviews are unavailable"
        stillWorks="Your history, evaluations and profile all work."
      />,
    );

    expect(screen.getByRole("status")).toHaveTextContent(/unavailable/i);
    expect(
      screen.getByText(/history, evaluations and profile/i),
    ).toBeInTheDocument();
  });
});

describe("every state, together", () => {
  it("has no axe violations across the set", async () => {
    const { container } = render(
      <div>
        <EmptyState
          title="Empty"
          action={<button type="button">Create</button>}
        >
          What will appear.
        </EmptyState>
        <PartialState
          missing="A section"
          action={<button type="button">Retry</button>}
        >
          <p>Loaded</p>
        </PartialState>
        <ForbiddenState what="A thing" grantedBy="an admin" />
        <ExpiredState
          what="A link"
          action={<button type="button">Renew</button>}
        />
        <DelayedState what="A job" />
        <InsufficientEvidenceState
          what="A competency"
          remedy="More sessions."
        />
        <UnassessableState
          what="A file"
          accepted="PDF."
          action={<button type="button">Replace</button>}
        />
        <ConnectionState phase="reconnecting" what="the session" />
        <DegradedState what="A capability" stillWorks="The rest." />
      </div>,
    );

    expect(await axe(container)).toHaveNoViolations();
  });
});
