import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

import { ApiError } from "@/lib/api/client";

import { CheckEmailPanel } from "./CheckEmailPanel";

/**
 * The check-email screen: the visible cooldown IAM-02 requires.
 *
 * Fake timers, because the countdown is the subject. userEvent gets the same
 * clock via advanceTimers or its internal waits would hang forever.
 */

describe("CheckEmailPanel", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  function renderPanel(request = vi.fn().mockResolvedValue(undefined)) {
    render(
      <CheckEmailPanel
        kind="password_reset"
        email="amara.eze@example.com"
        request={request}
        changeAddressHref="/forgot-password"
      />,
    );
    return request;
  }

  it("masks the address, because this screen can be read over a shoulder", () => {
    renderPanel();
    expect(screen.getByText("a•••••••e@example.com")).toBeInTheDocument();
    expect(screen.queryByText("amara.eze@example.com")).not.toBeInTheDocument();
  });

  it("counts the cooldown down and then enables the resend", async () => {
    renderPanel();

    const button = screen.getByRole("button", { name: /resend in 60s/i });
    expect(button).toBeDisabled();

    await act(async () => {
      vi.advanceTimersByTime(60_000);
    });

    expect(screen.getByRole("button", { name: /^resend email$/i })).toBeEnabled();
  });

  it("resends and restarts the cooldown", async () => {
    // fireEvent rather than userEvent: userEvent waits on real timers between
    // events, which the fake clock never advances, and the test hangs.
    const request = renderPanel();

    await act(async () => {
      vi.advanceTimersByTime(60_000);
    });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /^resend email$/i }));
    });

    expect(request).toHaveBeenCalledWith("password_reset", "amara.eze@example.com");
    expect(screen.getByText(/only the newest one works/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /resend in 60s/i })).toBeDisabled();
  });

  it("takes the server's cooldown over its own guess", async () => {
    // The countdown a person watches must be the cooldown that actually
    // holds, and only the 429 knows that number.
    const request = vi
      .fn()
      .mockRejectedValue(
        new ApiError({ status: 429, code: "RESEND_COOLING_DOWN", message: "wait", retryAfterSeconds: 17 }),
      );
    renderPanel(request);

    await act(async () => {
      vi.advanceTimersByTime(60_000);
    });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /^resend email$/i }));
    });

    expect(screen.getByRole("button", { name: /resend in 17s/i })).toBeDisabled();
  });

  it("says which email it was: subject and validity", () => {
    renderPanel();
    expect(screen.getByText("Set a new Prepeet password")).toBeInTheDocument();
    expect(screen.getByText(/30 minutes, and one use only/)).toBeInTheDocument();
  });

  it("has no accessibility violations", async () => {
    renderPanel();
    // axe drives its own timers; the fake clock would deadlock it. The region
    // rule is off because the panel renders alone here; on the page,
    // AuthShell's <main> is the landmark.
    vi.useRealTimers();
    expect(
      await axe(document.body, { rules: { region: { enabled: false } } }),
    ).toHaveNoViolations();
  });
});
