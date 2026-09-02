import type { components } from "@contracts";

import { apiFetch } from "@/lib/api/client";

/** How the workspace is configured, and whether this person may change it. */

export type TenantSettings = components["schemas"]["TenantSettings"];
export type TenantSettingsDocument =
  components["schemas"]["TenantSettingsDocument"];

/** The current configuration. Every membership role may read it. */
export async function getSettings(): Promise<TenantSettings> {
  return apiFetch<TenantSettings>("/tenant/settings");
}

/**
 * Save a change against the version it was made on.
 *
 * The version goes back exactly as it arrived. A save naming any other version
 * is refused rather than merged, so two administrators editing at once are told
 * they collided instead of one of them losing their work silently.
 */
export async function saveSettings(
  version: number,
  settings: TenantSettingsDocument,
): Promise<TenantSettings> {
  return apiFetch<TenantSettings>("/tenant/settings", {
    method: "PUT",
    body: { version, settings },
  });
}
