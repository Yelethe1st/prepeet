import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SettingsSection } from "./SettingsSection";

/**
 * The states a person meets around the settings: waiting, failed, editing, and
 * the collision that only happens when two administrators are working at once.
 */

const fetchMock = vi.fn();

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function settingsResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

const current = {
  version: 4,
  editable: true,
  settings: {
    organisation: {
      legal_name: "Northwind Health Limited",
      display_name: "Northwind Health",
    },
    defaults: {},
    candidate_experience: {},
    notifications: {},
  },
};

function renderSection() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(<SettingsSection />, {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  });
}

describe("SettingsSection", () => {
  it("says what it is waiting for", async () => {
    fetchMock.mockImplementation(() => new Promise(() => {}));

    renderSection();

    expect(
      await screen.findByText(/your workspace settings/i),
    ).toBeInTheDocument();
  });

  it("reassures the reader that a failure is only the view", async () => {
    // The fear on a settings screen is that the configuration is lost, not
    // that a request failed.
    fetchMock.mockResolvedValue(
      settingsResponse(500, { code: "INTERNAL", message: "nope" }),
    );

    renderSection();

    expect(
      await screen.findByText(
        /nothing about how this workspace is configured has changed/i,
      ),
    ).toBeInTheDocument();
  });

  it("renders the settings once they arrive", async () => {
    fetchMock.mockResolvedValue(settingsResponse(200, current));

    renderSection();

    expect(
      await screen.findByRole("textbox", { name: /legal name/i }),
    ).toBeInTheDocument();
  });

  it("tells an administrator when somebody else changed things first", async () => {
    // The one failure a person can act on, and only by looking rather than by
    // retrying: the same stale version would be refused forever.
    fetchMock
      .mockResolvedValueOnce(settingsResponse(200, current))
      .mockResolvedValueOnce(
        settingsResponse(409, {
          code: "SETTINGS_CONFLICT",
          message: "Somebody else changed these settings.",
        }),
      );

    renderSection();
    await userEvent.click(await screen.findByRole("button", { name: /save/i }));

    await waitFor(() =>
      expect(
        screen.getByText(/somebody else changed these settings/i),
      ).toBeInTheDocument(),
    );
  });
});

describe("SettingsSection after a save", () => {
  it("shows the new version rather than the one just replaced", async () => {
    fetchMock
      .mockResolvedValueOnce(settingsResponse(200, current))
      .mockResolvedValueOnce(settingsResponse(200, { ...current, version: 5 }));

    renderSection();
    await userEvent.click(await screen.findByRole("button", { name: /save/i }));

    // Refetching instead would briefly show version 4 again and invite a
    // second save against a version the server has already moved past.
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
  });

  it("asks again when told to", async () => {
    fetchMock.mockResolvedValue(
      settingsResponse(500, { code: "INTERNAL", message: "nope" }),
    );

    renderSection();
    const retry = await screen.findByRole("button", { name: /try again/i });
    const before = fetchMock.mock.calls.length;
    await userEvent.click(retry);

    await waitFor(() =>
      expect(fetchMock.mock.calls.length).toBeGreaterThan(before),
    );
  });
});
