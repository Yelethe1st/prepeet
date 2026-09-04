import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ReconnectionOverlay } from "./ReconnectionOverlay";

/**
 * The ported overlay's own obligations: an alertdialog whose attempt line
 * is a live region, truthfully worded in both of its states, with the two
 * decisions the person keeps.
 */

describe("the reconnection overlay", () => {
  it("names the attempt in a live region while the chain runs", () => {
    render(
      <ReconnectionOverlay
        phase={{ kind: "reconnecting", attempt: 2, maxAttempts: 5 }}
        onRetryNow={() => undefined}
        onEndInterview={() => undefined}
      />,
    );

    const dialog = screen.getByRole("alertdialog", {
      name: /reconnecting to the interview/i,
    });
    expect(dialog).toHaveTextContent(/paused and the timer has stopped/i);
    expect(screen.getByRole("status")).toHaveTextContent(
      /reconnection attempt 2 of 5/i,
    );
  });

  it("says plainly when automatic attempts are done", () => {
    render(
      <ReconnectionOverlay
        phase={{ kind: "exhausted", maxAttempts: 5 }}
        onRetryNow={() => undefined}
        onEndInterview={() => undefined}
      />,
    );

    expect(screen.getByRole("status")).toHaveTextContent(
      /automatic reconnection has stopped/i,
    );
  });

  it("takes focus on opening and returns it on recovery", () => {
    // A control the person was on when the connection dropped.
    const before = document.createElement("button");
    before.textContent = "End interview";
    document.body.appendChild(before);
    before.focus();

    const { unmount } = render(
      <ReconnectionOverlay
        phase={{ kind: "reconnecting", attempt: 1, maxAttempts: 5 }}
        onRetryNow={() => undefined}
        onEndInterview={() => undefined}
      />,
    );

    // The one decision the person can act on holds focus while the chain
    // runs; recovery closing the overlay puts them back where they were.
    expect(screen.getByRole("button", { name: /retry now/i })).toHaveFocus();

    unmount();
    expect(before).toHaveFocus();
    before.remove();
  });

  it("keeps both decisions in the person's hands", async () => {
    const retry = vi.fn();
    const end = vi.fn();
    const user = userEvent.setup();
    render(
      <ReconnectionOverlay
        phase={{ kind: "reconnecting", attempt: 1, maxAttempts: 5 }}
        onRetryNow={retry}
        onEndInterview={end}
      />,
    );

    await user.click(screen.getByRole("button", { name: /retry now/i }));
    await user.click(screen.getByRole("button", { name: /end interview/i }));

    expect(retry).toHaveBeenCalledTimes(1);
    expect(end).toHaveBeenCalledTimes(1);
  });
});
