"use client";

import { useState } from "react";

import { Banner } from "@/design-system/components";

/** One workspace somebody belongs to. */
export interface Membership {
  tenantId: string;
  tenantName: string;
  status: string;
}

export interface TenantSwitcherProps {
  memberships: Membership[];
  activeTenantId: string | null;
  /** Asks the server to switch. Injected so this is testable without one. */
  onSwitch: (tenantId: string) => Promise<void>;
}

/**
 * Choosing which workspace to act under.
 *
 * The control that makes an explicit active tenant usable. Everything about
 * which tenant a request runs under is decided on the server and stored on the
 * session, per ADR-0002 and IAM-03; this asks for a change and reflects the
 * answer, and is never the thing that decides.
 *
 * A select rather than a menu, deliberately. It is a choice between mutually
 * exclusive options with one currently in force, which is what a select is, and
 * it comes with keyboard behaviour, an accessible name and a mobile picker that
 * a custom menu would have to reimplement and would get partly wrong.
 */
export function TenantSwitcher({ memberships, activeTenantId, onSwitch }: TenantSwitcherProps) {
  const [switching, setSwitching] = useState(false);
  const [failed, setFailed] = useState(false);

  // Revoked memberships are listed by the server so an interface can explain
  // where a workspace went. They are not somewhere to act.
  const available = memberships.filter((membership) => membership.status !== "revoked");

  // One workspace is not a choice, and a control offering a single option only
  // invites a misclick. None is not a choice either.
  if (available.length < 2) {
    return null;
  }

  async function choose(tenantId: string) {
    // Refused while one is in flight. Two overlapping switches can settle in
    // either order, and the interface would then show the authority of one
    // workspace while the session is in the other.
    if (switching) return;

    setFailed(false);
    setSwitching(true);
    try {
      await onSwitch(tenantId);
    } catch {
      // Back to what the session actually is. Leaving the control showing the
      // attempted workspace would claim one the server never accepted.
      setFailed(true);
    } finally {
      setSwitching(false);
    }
  }

  return (
    <div>
      <label className="sr-only" htmlFor="workspace-switcher">
        Active workspace
      </label>
      <select
        className="select"
        id="workspace-switcher"
        value={activeTenantId ?? ""}
        disabled={switching}
        onChange={(event) => void choose(event.target.value)}
      >
        {activeTenantId === null ? <option value="">Choose a workspace</option> : null}
        {available.map((membership) => (
          <option key={membership.tenantId} value={membership.tenantId}>
            {membership.tenantName}
          </option>
        ))}
      </select>

      {failed ? (
        <Banner tone="danger">
          <strong>That workspace could not be opened.</strong> You are still in the one you were in.
        </Banner>
      ) : null}
    </div>
  );
}
