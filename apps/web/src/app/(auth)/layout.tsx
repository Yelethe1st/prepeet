import type { ReactNode } from "react";

import "@/design-system/base.css";
import "@/design-system/components.css";
import "@/design-system/layout.css";

/**
 * The authentication routes.
 *
 * The ported stylesheets are imported here rather than in the root layout so
 * that a route which does not use them does not pay for them. `body.auth` is
 * what layout.css keys the two-panel grid off, and Next has no way to set a
 * body class per route group, so the class is applied by this wrapper instead.
 */
export default function AuthLayout({ children }: { children: ReactNode }) {
  return <div className="auth">{children}</div>;
}
