import { afterEach, describe, expect, it, vi } from "vitest";

import { listDocuments, listFacts, reviewFact, uploadCv } from "./api";
import { apiFetch } from "@/lib/api/client";

vi.mock("@/lib/api/client", () => ({
  apiFetch: vi.fn(),
}));

/**
 * The upload path, which is the one call sequence in this feature with an
 * order that matters: start allocates the version and presigns, the browser
 * itself PUTs the bytes, complete records the digest. A failure between the
 * PUT and complete must abort, because a version left silently uploading is
 * the invisible stall PRO-02 gave its own state to.
 */

const fetchMock = vi.fn();
vi.stubGlobal("fetch", fetchMock);

// jsdom's File has no arrayBuffer; the FileReader it does have provides one.
if (!File.prototype.arrayBuffer) {
  File.prototype.arrayBuffer = function arrayBuffer(this: File) {
    return new Promise<ArrayBuffer>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result as ArrayBuffer);
      reader.onerror = () => reject(reader.error);
      reader.readAsArrayBuffer(this);
    });
  };
}

const started = {
  document: { id: "d1", version: 1, state: "uploading" },
  upload_id: "u1",
  part_urls: ["https://bucket.example/part-1"],
  expires_at: "2026-08-25T11:00:00Z",
};

afterEach(() => {
  vi.mocked(apiFetch).mockReset();
  fetchMock.mockReset();
});

describe("uploadCv", () => {
  it("starts, PUTs the bytes itself, and completes with the real digest", async () => {
    const body = new Uint8Array([1, 2, 3]);
    const file = new File([body], "cv.txt", { type: "text/plain" });
    vi.mocked(apiFetch)
      .mockResolvedValueOnce(started)
      .mockResolvedValueOnce({ id: "d1", state: "stored" });
    fetchMock.mockResolvedValueOnce({
      ok: true,
      headers: new Headers({ ETag: '"etag-1"' }),
    });

    const stored = await uploadCv(file);

    expect(stored).toEqual({ id: "d1", state: "stored" });
    // The start names the file's own type and size.
    expect(vi.mocked(apiFetch).mock.calls[0]).toEqual([
      "/me/documents",
      {
        method: "POST",
        body: { media_type: "text/plain", size_bytes: 3, part_count: 1 },
      },
    ]);
    // The bytes went to the presigned URL, not to the API.
    expect(fetchMock).toHaveBeenCalledWith(
      "https://bucket.example/part-1",
      expect.objectContaining({ method: "PUT" }),
    );
    // The digest is the content's real SHA-256, not a placeholder.
    const digest =
      "039058c6f2c0cb492c533b0a4d14ef77cc0f78abccced5287d84a1a2011cfb81";
    expect(vi.mocked(apiFetch).mock.calls[1]).toEqual([
      "/me/documents/d1/complete",
      {
        method: "POST",
        body: {
          upload_id: "u1",
          sha256: digest,
          size_bytes: 3,
          parts: [{ number: 1, etag: '"etag-1"' }],
        },
      },
    ]);
  });

  it("aborts the upload when the PUT fails, so the stall is visible rather than silent", async () => {
    const file = new File([new Uint8Array([1])], "cv.pdf", {
      type: "application/pdf",
    });
    vi.mocked(apiFetch).mockResolvedValue(started);
    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 500,
      headers: new Headers(),
    });

    await expect(uploadCv(file)).rejects.toThrow();

    expect(vi.mocked(apiFetch)).toHaveBeenCalledWith("/me/documents/d1/abort", {
      method: "POST",
    });
  });
});

describe("the profile reads", () => {
  it("unwraps the document and fact envelopes", async () => {
    const documents = [{ id: "d1" }];
    vi.mocked(apiFetch).mockResolvedValue({ documents } as never);
    await expect(listDocuments()).resolves.toBe(documents);
    expect(apiFetch).toHaveBeenCalledWith("/me/documents");

    const facts = [{ id: "f1" }];
    vi.mocked(apiFetch).mockResolvedValue({ facts } as never);
    await expect(listFacts("d1")).resolves.toBe(facts);
    expect(apiFetch).toHaveBeenLastCalledWith("/me/documents/d1/facts");
  });
});

describe("reviewFact", () => {
  it("posts the candidate's decision to the fact's own review", async () => {
    vi.mocked(apiFetch).mockResolvedValue({ id: "f1" } as never);

    await reviewFact("f1", {
      status: "corrected",
      corrected_value: { title: "Engineer" },
    });

    expect(apiFetch).toHaveBeenCalledWith("/me/facts/f1/review", {
      method: "POST",
      body: { status: "corrected", corrected_value: { title: "Engineer" } },
    });
  });
});
