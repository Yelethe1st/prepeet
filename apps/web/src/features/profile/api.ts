import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

import type { Document, Fact } from "./facts";

/**
 * The profile's calls, typed from the contract and living beside the feature
 * per the client's own rule: the client grows no method per endpoint.
 */

type DocumentList = components["schemas"]["DocumentList"];
type FactList = components["schemas"]["FactList"];
type StartedUpload = components["schemas"]["StartedUpload"];
export type ReviewFactRequest = components["schemas"]["ReviewFactRequest"];

/** Every version of the CV, states included. */
export async function listDocuments(): Promise<Document[]> {
  const list = await apiFetch<DocumentList>("/me/documents");
  return list.documents;
}

/** One document's extraction, spans included. */
export async function listFacts(documentId: string): Promise<Fact[]> {
  const list = await apiFetch<FactList>(`/me/documents/${documentId}/facts`);
  return list.facts;
}

/** The candidate's confirm, correct or reject on one fact. */
export async function reviewFact(
  factId: string,
  review: ReviewFactRequest,
): Promise<Fact> {
  return apiFetch<Fact>(`/me/facts/${factId}/review`, {
    method: "POST",
    body: review,
  });
}

/**
 * uploadCv carries one CV up, browser-direct.
 *
 * The order is the contract's: start allocates the version and presigns, the
 * browser PUTs the bytes at the bucket itself - no credential, no proxy -
 * and complete records the digest this file actually hashes to, which is the
 * identity extraction pins. A failed PUT aborts, so the version reads failed
 * visibly instead of sitting in uploading forever.
 */
export async function uploadCv(file: File): Promise<Document> {
  const started = await apiFetch<StartedUpload>("/me/documents", {
    method: "POST",
    body: { media_type: file.type, size_bytes: file.size, part_count: 1 },
  });

  try {
    const bytes = await file.arrayBuffer();
    const response = await fetch(started.part_urls[0] ?? "", {
      method: "PUT",
      body: bytes,
    });
    if (!response.ok) {
      throw new Error(`The upload was refused with status ${response.status}.`);
    }

    const hash = await crypto.subtle.digest("SHA-256", bytes);
    const sha256 = [...new Uint8Array(hash)]
      .map((b) => b.toString(16).padStart(2, "0"))
      .join("");

    return await apiFetch<Document>(
      `/me/documents/${started.document.id}/complete`,
      {
        method: "POST",
        body: {
          upload_id: started.upload_id,
          sha256,
          size_bytes: file.size,
          parts: [{ number: 1, etag: response.headers.get("ETag") ?? "" }],
        },
      },
    );
  } catch (error) {
    // Best effort: the abort itself failing must not mask why we are here.
    await apiFetch(`/me/documents/${started.document.id}/abort`, {
      method: "POST",
    }).catch(() => undefined);
    throw error;
  }
}
