"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useState } from "react";
import type { ReactNode } from "react";

import { Button } from "@/shared/components";

import { TenantSwitcher } from "./TenantSwitcher";
import { visibleNavigation } from "./navigation";

/**
 * What the shell needs to know about who is signed in.
 *
 * Declared here rather than imported from the auth feature, because a feature
 * importing another feature's types is how two features become one. The route
 * layout supplies it, which is the composition root's job: `app/` is allowed to
 * see everything, in the way `cmd/` is on the server.
 *
 * It is also less than the session carries. The shell needs an address to show,
 * workspaces to switch between and capabilities to render from, and nothing
 * else, so anything added to a session does not become something the shell can
 * quietly start depending on.
 */
export interface ShellUser {
  email: string;
  activeTenantId: string | null;
  memberships: { tenantId: string; tenantName: string; status: string }[];
  capabilities: string[];
}

export interface AppShellProps {
  user: ShellUser;
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
export function AppShell({
  user,
  onSignOut,
  onSwitchTenant,
  children,
}: AppShellProps) {
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
    <div className="flex min-h-screen">
      <a className="skip-link" href="#main-content">
        Skip to main content
      </a>

      {/*
        The scrim closes the menu on a tap outside it. It is not focusable and
        carries no label, because it duplicates the close button rather than
        adding a destination, and announcing it would put an unnamed control in
        everybody's way.
      */}
      {menuOpen ? (
        <div
          className="fixed inset-0 z-[70] bg-overlay lg:hidden"
          onClick={() => setMenuOpen(false)}
          aria-hidden="true"
        />
      ) : null}

      {/*
        Off-canvas below lg, exactly as the prototype hides it below 1024px. If
        it did not move out of the way the content would be pushed off the right
        of a small screen and every page would be unusable rather than cramped.
      */}
      <aside
        className={[
          "fixed inset-y-0 left-0 z-[80] flex w-[min(84vw,300px)] flex-col",
          "border-r border-sidebar-border bg-sidebar-bg transition-transform",
          menuOpen ? "translate-x-0 shadow-lg" : "-translate-x-full",
          "lg:sticky lg:top-0 lg:h-screen lg:w-sidebar lg:translate-x-0 lg:shadow-none",
        ].join(" ")}
      >
        <Link
          className="flex h-topbar items-center gap-2.5 border-b border-sidebar-border px-4 text-fg no-underline"
          href="/"
        >
          <span
            className="h-[30px] w-[30px] flex-none rounded-[9px] bg-primary"
            aria-hidden="true"
          />
          <span className="text-[17px] font-bold tracking-tight">Prepeet</span>
        </Link>

        <nav
          className="flex flex-1 flex-col gap-[18px] overflow-y-auto p-3"
          id="main-navigation"
          aria-label="Main"
        >
          {groups.map((group) => (
            <div key={group.label}>
              <p className="px-2.5 pb-1.5 text-2xs font-bold tracking-[0.1em] text-fg-3 uppercase">
                {group.label}
              </p>
              {group.items.map((item) => (
                <Link
                  key={item.href}
                  className={
                    "flex items-center gap-2.5 rounded-md px-2.5 py-2 text-sm font-medium " +
                    "text-sidebar-fg no-underline transition-colors hover:bg-surface-3 hover:text-fg " +
                    "aria-[current=page]:bg-sidebar-active aria-[current=page]:font-semibold " +
                    "aria-[current=page]:text-sidebar-fg-active"
                  }
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

        <div className="flex flex-col gap-1 border-t border-sidebar-border p-2.5">
          <span className="truncate px-2.5 text-sm font-semibold text-fg">
            {user.email}
          </span>
          <Button variant="ghost" size="sm" onClick={() => void signOut()}>
            Sign out
          </Button>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header
          className={
            "sticky top-0 z-50 flex h-topbar items-center gap-2.5 border-b border-border " +
            "bg-bg/90 px-4 backdrop-blur sm:px-6"
          }
        >
          {/*
            Hidden once the sidebar is permanently visible, as the prototype
            hides it above 1024px. A button that opens something already open is
            a control that does nothing, and it would still be in the tab order.
          */}
          <span className="lg:hidden">
            <Button
              variant="ghost"
              size="sm"
              aria-expanded={menuOpen}
              aria-controls="main-navigation"
              onClick={() => setMenuOpen((open) => !open)}
            >
              {menuOpen ? "Close menu" : "Menu"}
            </Button>
          </span>

          <div className="ml-auto flex items-center gap-1">
            <TenantSwitcher
              memberships={user.memberships}
              activeTenantId={user.activeTenantId}
              onSwitch={onSwitchTenant}
            />
          </div>
        </header>

        <main
          className="mx-auto w-full max-w-content flex-1 px-4 pt-6 pb-20 sm:px-6 sm:pb-12"
          id="main-content"
        >
          {children}
        </main>
      </div>
    </div>
  );
}
