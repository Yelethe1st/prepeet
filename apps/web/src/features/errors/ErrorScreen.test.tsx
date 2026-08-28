import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { axe } from "vitest-axe";

import { ErrorScreen } from "./ErrorScreen";

describe("ErrorScreen", () => {
  function renderScreen() {
    return render(
      <ErrorScreen
        badge="403 · permission denied"
        title="You cannot open this page"
        actions={<a href="/practice">Back</a>}
        facts={[
          { label: "Reference", value: "req_9", mono: true },
          { label: "Decision", value: "Refused." },
        ]}
        factsTitle="What was refused"
      >
        <p>Nothing has gone wrong with your account.</p>
      </ErrorScreen>,
    );
  }

  it("puts the badge, title, explanation, action and facts on screen", () => {
    renderScreen();

    expect(screen.getByText("403 · permission denied")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: /cannot open/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/nothing has gone wrong/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Back" })).toBeInTheDocument();
    expect(screen.getByText("req_9")).toBeInTheDocument();
  });

  it("renders no facts section when there are none to state", () => {
    render(
      <ErrorScreen badge="b" title="t" actions={<a href="/practice">a</a>}>
        <p>body</p>
      </ErrorScreen>,
    );
    expect(screen.queryByRole("heading", { level: 2 })).not.toBeInTheDocument();
  });

  it("is itself the main landmark, since these pages own the whole viewport", () => {
    renderScreen();
    expect(screen.getByRole("main")).toBeInTheDocument();
  });

  it("has no accessibility violations", async () => {
    const { container } = renderScreen();
    expect(await axe(container)).toHaveNoViolations();
  });
});
