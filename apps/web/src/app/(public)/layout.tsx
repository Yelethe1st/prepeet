import type { ReactNode } from "react";

/**
 * The routes somebody reaches without a session.
 *
 * Grouped by audience per the repository structure in the architecture brief:
 * public, candidate, recruiter, platform. Signing in and registering are public
 * because that is who reaches them, which is a more useful division than
 * "authenticated or not" once there are marketing pages here too.
 *
 * It adds no markup of its own. Each screen owns its own layout, and the
 * two-panel arrangement the authentication screens share is a component they
 * both use rather than something imposed on everything in this group.
 */
export default function PublicLayout({ children }: { children: ReactNode }) {
  return children;
}
