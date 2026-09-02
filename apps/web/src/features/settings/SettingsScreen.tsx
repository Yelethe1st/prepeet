"use client";

import { useState } from "react";

import { Button } from "@/shared/components";

import type { TenantSettings, TenantSettingsDocument } from "./api";

/**
 * How the workspace is configured, editable or not.
 *
 * TEN-01's criterion is that a read-only member sees the settings without
 * controls rather than a broken form. Two things follow from that, and both are
 * easy to get wrong in a way nobody notices.
 *
 * The absence of controls is explained. A page with no inputs and no reason
 * reads as something that failed to load; naming the authority that is missing
 * turns it into a boundary somebody can act on, by asking the right person.
 *
 * And whether to draw controls comes from the server, not from a role name the
 * browser interprets. A second copy of the authorization rules living here
 * would drift from the first, and the copy that drifts is the one nobody
 * re-reads.
 */
export function SettingsScreen({
  settings,
  onSave,
  saving = false,
  conflicted = false,
}: {
  settings: TenantSettings;
  onSave: (version: number, document: TenantSettingsDocument) => void;
  saving?: boolean;
  conflicted?: boolean;
}) {
  const [draft, setDraft] = useState(settings.settings);

  return (
    <section>
      {conflicted && (
        <p role="alert">
          Somebody else changed these settings while you were editing. Reload to
          see the current values before saving again.
        </p>
      )}

      <h2>Organisation</h2>
      {settings.editable ? (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            // The version that was read, not a fresh one. Sending anything
            // else would make the collision check always agree with itself.
            onSave(settings.version, draft);
          }}
        >
          <Field
            label="Legal name"
            hint="The entity that answers for the hiring decision."
            value={draft.organisation.legal_name}
            onChange={(legal_name) =>
              setDraft({
                ...draft,
                organisation: { ...draft.organisation, legal_name },
              })
            }
          />
          <Field
            label="Display name"
            hint="What a candidate sees on an invitation."
            value={draft.organisation.display_name}
            onChange={(display_name) =>
              setDraft({
                ...draft,
                organisation: { ...draft.organisation, display_name },
              })
            }
          />
          <Button type="submit" busy={saving} busyLabel="Saving…">
            Save
          </Button>
        </form>
      ) : (
        <>
          <dl>
            <Value
              label="Legal name"
              value={settings.settings.organisation.legal_name}
            />
            <Value
              label="Display name"
              value={settings.settings.organisation.display_name}
            />
          </dl>
          <p>
            You can see how this workspace is configured but not change it. An
            administrator or owner can change these.
          </p>
        </>
      )}

      {/* Version zero is the platform defaults, which nobody chose, so there
          is no change to describe and claiming one would be an invention. */}
      {settings.version > 0 && settings.changed_at !== undefined && (
        <p>Last changed {formatChanged(settings.changed_at)}.</p>
      )}
    </section>
  );
}

function Field({
  label,
  hint,
  value,
  onChange,
}: {
  label: string;
  hint: string;
  value: string;
  onChange: (next: string) => void;
}) {
  return (
    <p>
      <label>
        {label}
        <input
          type="text"
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      </label>
      <span>{hint}</span>
    </p>
  );
}

function Value({ label, value }: { label: string; value: string }) {
  return (
    <>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </>
  );
}

/**
 * A date a person reads, with the month named.
 *
 * Numeric dates mean two different days depending on where the reader is, and
 * a configuration history is exactly where that ambiguity costs something.
 */
function formatChanged(changedAt: string): string {
  return new Date(changedAt).toLocaleDateString("en-GB", {
    day: "numeric",
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  });
}
