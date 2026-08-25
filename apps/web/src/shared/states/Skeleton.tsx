import type { ReactNode } from "react";

/**
 * Loading, as the shape of what is coming rather than a spinner.
 *
 * The shapes are the prototype's skeleton vocabulary; each surface composes
 * them into a mirror of its own layout, which is the ticket's rule that a
 * skeleton matches the content it replaces. What the wrapper owns is the part
 * every surface would otherwise get wrong on its own: the loading state is
 * announced once, by name, and the shapes themselves are hidden from
 * assistive technology - a page of unlabeled grey boxes read aloud is a page
 * of nothing, repeated.
 */
export function LoadingSurface({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div role="status" aria-busy="true">
      <span className="sr-only">Loading {label}</span>
      <div aria-hidden="true">{children}</div>
    </div>
  );
}

/** The prototype's width steps for a pending line of copy. */
export type SkeletonTextWidth = "full" | "75" | "50" | "25";

const textWidths: Record<SkeletonTextWidth, string> = {
  full: "w-full",
  "75": "w-3/4",
  "50": "w-1/2",
  "25": "w-1/4",
};

/** One line of pending text: 12px tall, in the prototype's width steps. */
export function SkeletonText({
  width = "full",
}: {
  width?: SkeletonTextWidth;
}) {
  return <div className={`skeleton my-1.5 h-3 ${textWidths[width]}`} />;
}

/** A pending block: a card body, a chart, a table region. */
export function SkeletonBlock({
  className = "h-[120px]",
}: {
  className?: string;
}) {
  return <div className={`skeleton rounded-md ${className}`} />;
}

/** A pending avatar or icon. */
export function SkeletonCircle({
  className = "size-10",
}: {
  className?: string;
}) {
  return <div className={`skeleton rounded-full ${className}`} />;
}
