"use client";

import * as Select from "@radix-ui/react-select";
import { useState } from "react";

import { Banner } from "@/shared/components";

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
 * Radix rather than a native select, per the technology baseline. What that
 * buys is the part a styled dropdown usually loses: focus is trapped and
 * restored, typeahead works, the trigger and the listbox are wired together for
 * assistive technology, and it can be styled without any of that being
 * reimplemented.
 */
export function TenantSwitcher({
  memberships,
  activeTenantId,
  onSwitch,
}: TenantSwitcherProps) {
  const [switching, setSwitching] = useState(false);
  const [failed, setFailed] = useState(false);

  // Revoked memberships are listed by the server so an interface can explain
  // where a workspace went. They are not somewhere to act.
  const available = memberships.filter(
    (membership) => membership.status !== "revoked",
  );

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

  const active = available.find(
    (membership) => membership.tenantId === activeTenantId,
  );

  return (
    <div>
      <Select.Root
        value={activeTenantId ?? undefined}
        disabled={switching}
        onValueChange={(value) => void choose(value)}
      >
        <Select.Trigger
          className={
            // Bounded and truncating. A workspace name is user supplied and can
            // be long, and the native select this replaced shrank on its own
            // where a Radix trigger does not: at 320px with text at 200% the
            // topbar was 357px wide and the page scrolled sideways. Caught by
            // the browser suite, which is the only tier that measures.
            "inline-flex min-h-8 max-w-[45vw] items-center gap-2 overflow-hidden rounded-md " +
            "border border-border bg-surface px-3 text-sm whitespace-nowrap text-fg " +
            "transition-colors hover:border-fg-muted disabled:cursor-not-allowed " +
            "disabled:opacity-60 lg:max-w-[16rem]"
          }
          aria-label="Active workspace"
        >
          <span className="truncate">
            <Select.Value placeholder="Choose a workspace">
              {active?.tenantName}
            </Select.Value>
          </span>
          <Select.Icon className="flex-none" aria-hidden="true">
            ▾
          </Select.Icon>
        </Select.Trigger>

        <Select.Portal>
          <Select.Content
            className="overflow-hidden rounded-md border border-border bg-surface shadow-lg"
            position="popper"
            sideOffset={4}
          >
            <Select.Viewport className="p-1">
              {available.map((membership) => (
                <Select.Item
                  key={membership.tenantId}
                  value={membership.tenantId}
                  className={
                    "cursor-pointer rounded-sm px-3 py-2 text-sm text-fg outline-none " +
                    "data-[highlighted]:bg-surface-3 data-[state=checked]:font-semibold"
                  }
                >
                  <Select.ItemText>{membership.tenantName}</Select.ItemText>
                </Select.Item>
              ))}
            </Select.Viewport>
          </Select.Content>
        </Select.Portal>
      </Select.Root>

      {failed ? (
        <Banner tone="danger">
          <strong>That workspace could not be opened.</strong> You are still in
          the one you were in.
        </Banner>
      ) : null}
    </div>
  );
}
