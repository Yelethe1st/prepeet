"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useState } from "react";
import type { ReactNode } from "react";

import type { SessionUser } from "@/features/auth/session";

import { TenantSwitcher } from "./TenantSwitcher";
import { visibleNavigation } from "./navigation";

export interface AppShellProps {
  user: SessionUser;
  onSignOut: () => Promise<void>;
  onSwitchTenant: (tenantId: string) => Promise<void>;
  children: ReactNode;
}

/**
 * The application shell, ported from the admin screens in /screens.
 *
 * Sidebar, topbar and content, with the sidebar off-canvas below 1024px. The
 * class names and the responsive behaviour are the prototype's; what is here is
 * the part markup cannot express: which destinations this session is offered,
 * and what happens when it leaves.
 *
 * # Navigation is offered, never enforced
 *
 * The items come from the capabilities the session holds, so nobody is shown a
 * control that will refuse them. That is a courtesy and not a boundary: the
 * server authorises every request against the same capabilities, so somebody
 * typing an address they cannot use is refused there. Treating this as the
 * protection is how a product ends up with an unguarded endpoint behind a
 * hidden button.
 */
export function AppShell({ user, onSignOut, onSwitchTenant, children }: AppShellProps) {
  const router = useRouter();
  const pathname = usePathname();
  const [menuOpen, setMenuOpen] = useState(false);
  const [signingOut, setSigningOut] = useState(false);

  const groups = visibleNavigation(user.capabilities);

  async function signOut() {
    if (signingOut) return;
    setSigningOut(true);

    try {
      await onSignOut();
    } catch {
      // Deliberately swallowed. Either the session was revoked or it was not,
      // and leaving somebody on an authenticated screen because the request
      // failed is the worse of the two outcomes: they believe they signed out.
      // The cookies are cleared by the response when there is one, and the next
      // request fails closed when there is not.
    } finally {
      router.push("/login");
    }
  }

  return (
    <div className={`app${menuOpen ? " sidebar-open" : ""}`}>
      <a className="skip-link" href="#main-content">
        Skip to main content
      </a>

      {/*
        The scrim closes the menu on a tap outside it. It is not focusable and
        carries no label, because it duplicates the close button rather than
        adding a destination, and announcing it would put an unnamed control in
        everybody's way.
      */}
      <div className="sidebar-scrim" onClick={() => setMenuOpen(false)} aria-hidden="true" />

      <aside className="sidebar">
        <Link className="sidebar-brand" href="/">
          <span className="logo-mark" aria-hidden="true" />
          <span className="wordmark">Prepeet</span>
        </Link>

        <nav className="sidebar-nav" id="main-navigation" aria-label="Main">
          {groups.map((group) => (
            <div key={group.label}>
              <p className="nav-group-label">{group.label}</p>
              {group.items.map((item) => (
                <Link
                  key={item.href}
                  className="nav-item"
                  href={item.href}
                  // aria-current rather than a class alone, so the current
                  // destination is announced and not only coloured.
                  aria-current={pathname === item.href ? "page" : undefined}
                  onClick={() => setMenuOpen(false)}
                >
                  <span>{item.label}</span>
                </Link>
              ))}
            </div>
          ))}
        </nav>

        <div className="sidebar-foot">
          <div className="sidebar-user">
            <div>
              <span className="u-name">{user.email}</span>
            </div>
          </div>
          <button className="btn btn-ghost btn-sm" type="button" onClick={() => void signOut()}>
            Sign out
          </button>
        </div>
      </aside>

      <div className="app-main">
        <header className="topbar">
          <button
            className="icon-btn menu-toggle"
            type="button"
            aria-expanded={menuOpen}
            aria-controls="main-navigation"
            onClick={() => setMenuOpen((open) => !open)}
          >
            {menuOpen ? "Close menu" : "Menu"}
          </button>

          <div className="topbar-actions">
            <TenantSwitcher
              memberships={user.memberships}
              activeTenantId={user.activeTenantId}
              onSwitch={onSwitchTenant}
            />
          </div>
        </header>

        <main className="app-content" id="main-content">
          {children}
        </main>
      </div>
    </div>
  );
}
