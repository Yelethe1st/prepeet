import type { ReactNode } from "react";

/** The banner tones the prototype defines. */
export type BannerTone = "success" | "danger" | "warning" | "info";

export interface BannerProps {
  tone: BannerTone;
  children: ReactNode;
}

const tones: Record<BannerTone, string> = {
  success: "bg-success-soft text-success-fg border-success-border",
  danger: "bg-danger-soft text-danger-fg border-danger-border",
  warning: "bg-warning-soft text-warning-fg border-warning-border",
  info: "bg-info-soft text-info-fg border-info-border",
};

/**
 * A page-level message, ported from the `.banner` block in the prototype.
 *
 * The role is chosen from the tone rather than passed in, because the two must
 * agree and a prop invites them not to. A failure is an alert, which is
 * announced immediately and interrupts; anything else is a status, which waits
 * for a pause. Announcing a success as an alert trains people to ignore alerts.
 */
export function Banner({ tone, children }: BannerProps) {
  return (
    <div
      className={`mb-[18px] flex gap-3 rounded-md border px-4 py-3 text-sm ${tones[tone]}`}
      role={tone === "danger" ? "alert" : "status"}
    >
      <div>{children}</div>
    </div>
  );
}
