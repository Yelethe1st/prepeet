import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { axe } from "vitest-axe";

import { QueryProvider } from "@/lib/api/QueryProvider";
import { ApiError } from "@/lib/api/client";

import { CvSection } from "./CvSection";
import * as api from "./api";
import type { Document, Fact } from "./facts";

vi.mock("./api");

/**
 * The review surface, from the outside: what the candidate sees about each
 * fact, and what their acts send to the server. PRO-04's first box is the
 * anchor - every fact shows its source span, its confidence and whether it
 * has been corrected - and the degradation states are PRO-03's, made visible.
 */

const storedCv: Document = {
  id: "d1",
  kind: "cv",
  version: 2,
  media_type: "text/plain",
  size_bytes: 1200,
  state: "stored",
  extraction_state: "extracted",
  created_at: "2026-08-20T10:00:00Z",
} as Document;

const skill: Fact = {
  id: "f1",
  document_id: "d1",
  kind: "skill",
  value: { name: "Go", confidence: 0.8 },
  span_start: 80,
  span_end: 82,
  confidence: 0.8,
  extractor_version: "extract-1",
  status: "proposed",
  created_at: "2026-08-20T10:00:00Z",
} as Fact;

const unparsed: Fact = {
  id: "f2",
  document_id: "d1",
  kind: "unparsed",
  value: { text: "I also volunteer at a coding club", confidence: 0 },
  span_start: 90,
  span_end: 140,
  confidence: 0,
  extractor_version: "extract-1",
  status: "proposed",
  created_at: "2026-08-20T10:00:00Z",
} as Fact;

function renderSection(): ReturnType<typeof render> {
  return render(<CvSection />, {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryProvider>{children}</QueryProvider>
    ),
  });
}

afterEach(() => {
  vi.mocked(api.listDocuments).mockReset();
  vi.mocked(api.listFacts).mockReset();
  vi.mocked(api.reviewFact).mockReset();
  vi.mocked(api.uploadCv).mockReset();
});

describe("the CV states", () => {
  it("announces loading by name before anything is known", () => {
    vi.mocked(api.listDocuments).mockReturnValue(new Promise(() => {}));

    renderSection();

    expect(screen.getByRole("status")).toHaveTextContent(/loading your cv/i);
  });

  it("offers the first upload when no CV is stored", async () => {
    vi.mocked(api.listDocuments).mockResolvedValue([]);

    renderSection();

    expect(
      await screen.findByRole("heading", { name: /no cv yet/i }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/upload your cv/i)).toBeInTheDocument();
  });

  it("says the reading is still running while extraction is pending, without blocking", async () => {
    vi.mocked(api.listDocuments).mockResolvedValue([
      { ...storedCv, extraction_state: "pending" },
    ]);

    renderSection();

    expect(
      await screen.findByRole("heading", { name: /still running/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/nothing is lost/i)).toBeInTheDocument();
  });

  it("says a format could not be read and offers a different file, never an error", async () => {
    vi.mocked(api.listDocuments).mockResolvedValue([
      {
        ...storedCv,
        extraction_state: "unsupported",
        media_type: "application/pdf",
      },
    ]);

    renderSection();

    expect(
      await screen.findByRole("heading", { name: /could not read/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/plain text/i)).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("reports a load failure with what is safe, the retry and the reference", async () => {
    vi.mocked(api.listDocuments).mockRejectedValue(
      new ApiError({ status: 500, message: "It broke.", requestId: "req_9" }),
    );

    renderSection();

    // The query retries a server failure twice before settling, by design.
    expect(
      await screen.findByRole("alert", {}, { timeout: 8000 }),
    ).toHaveTextContent(/could not be loaded/i);
    expect(screen.getByText("req_9")).toBeInTheDocument();
    const retry = screen.getByRole("button", { name: /try again/i });
    expect(retry).toBeInTheDocument();

    // The retry is the way back: taking it asks again and the section
    // recovers, which is what makes the offer worth making.
    const asked = vi.mocked(api.listDocuments).mock.calls.length;
    vi.mocked(api.listDocuments).mockResolvedValue([]);
    vi.mocked(api.listFacts).mockResolvedValue([]);
    await userEvent.setup().click(retry);

    await waitFor(() =>
      expect(vi.mocked(api.listDocuments).mock.calls.length).toBeGreaterThan(
        asked,
      ),
    );
    expect(
      await screen.findByRole("heading", { name: /no cv yet/i }),
    ).toBeInTheDocument();
  });
});

describe("the facts", () => {
  function withFacts(facts: Fact[]): void {
    vi.mocked(api.listDocuments).mockResolvedValue([storedCv]);
    vi.mocked(api.listFacts).mockResolvedValue(facts);
  }

  it("shows each fact with its confidence, source span and review state", async () => {
    withFacts([skill, unparsed]);

    renderSection();

    const row = within(await screen.findByRole("listitem", { name: /go/i }));
    expect(row.getByText("80% confident")).toBeInTheDocument();
    expect(
      row.getByText(/characters 80 to 82 of your cv/i),
    ).toBeInTheDocument();
    expect(row.getByText("Parsed")).toBeInTheDocument();
  });

  it("surfaces what could not be parsed instead of dropping it", async () => {
    withFacts([skill, unparsed]);

    renderSection();

    expect(await screen.findByText(/could not parse/i)).toBeInTheDocument();
    expect(screen.getByText(/volunteer at a coding club/i)).toBeInTheDocument();
  });

  it("confirming sends the move and shows the new state", async () => {
    withFacts([skill]);
    vi.mocked(api.reviewFact).mockResolvedValue({
      ...skill,
      status: "confirmed",
    });
    const user = userEvent.setup();

    renderSection();
    await user.click(await screen.findByRole("button", { name: /confirm/i }));

    expect(api.reviewFact).toHaveBeenCalledWith("f1", { status: "confirmed" });
    expect(await screen.findByText("Confirmed by you")).toBeInTheDocument();
  });

  it("editing sends the correction and keeps the original visible", async () => {
    withFacts([skill]);
    vi.mocked(api.reviewFact).mockResolvedValue({
      ...skill,
      status: "corrected",
      corrected_value: { name: "Golang" },
    });
    const user = userEvent.setup();

    renderSection();
    await user.click(await screen.findByRole("button", { name: /edit/i }));
    const input = screen.getByLabelText(/your version/i);
    await user.clear(input);
    await user.type(input, "Golang");
    await user.click(screen.getByRole("button", { name: /save/i }));

    // The correction carries the candidate's whole version, confidence gone.
    expect(api.reviewFact).toHaveBeenCalledWith("f1", {
      status: "corrected",
      corrected_value: { name: "Golang" },
    });
    expect(await screen.findByText("Edited by you")).toBeInTheDocument();
    // The original extraction stays on the page - nothing was destroyed.
    expect(screen.getByText(/parsed as/i)).toHaveTextContent(/go/i);
  });

  it("rejecting is reversible, because it is a status and not a deletion", async () => {
    withFacts([skill]);
    vi.mocked(api.reviewFact).mockResolvedValue({
      ...skill,
      status: "rejected",
    });
    const user = userEvent.setup();

    renderSection();
    await user.click(await screen.findByRole("button", { name: /reject/i }));

    expect(api.reviewFact).toHaveBeenCalledWith("f1", { status: "rejected" });
    expect(await screen.findByText("Rejected by you")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /restore/i }),
    ).toBeInTheDocument();
  });

  it("has no axe violations with facts on the page", async () => {
    withFacts([skill, unparsed]);

    const { container } = renderSection();
    await screen.findByText("80% confident");

    expect(await axe(container)).toHaveNoViolations();
  });
});

describe("uploading", () => {
  it("uploads the chosen file and refreshes", async () => {
    vi.mocked(api.listDocuments)
      .mockResolvedValueOnce([])
      .mockResolvedValue([storedCv]);
    vi.mocked(api.listFacts).mockResolvedValue([]);
    vi.mocked(api.uploadCv).mockResolvedValue(storedCv);
    const user = userEvent.setup();

    renderSection();
    const file = new File(["hello"], "cv.txt", { type: "text/plain" });
    await user.upload(await screen.findByLabelText(/upload your cv/i), file);

    expect(api.uploadCv).toHaveBeenCalledWith(file);
    await waitFor(() => expect(api.listDocuments).toHaveBeenCalledTimes(2));
  });
});
