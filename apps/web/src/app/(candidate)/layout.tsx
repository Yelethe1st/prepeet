"use client";

import type { ReactNode } from "react";

import "@/shared/styles/theme.css";
import { useRouter } from "next/navigation";
import { useEffect } from "react";

import { SessionProvider, useSession } from "@/lib/auth/session";
import { setActiveTenant, signOut } from "@/lib/auth/api";
import { AppShell } from "@/features/shell/AppShell";

/**
 * Everything behind a session.
 *
 * The shell is here rather than in each page so that navigation, the workspace
 * switcher and signing out exist once. A page under this route group renders its
 * own content and nothing else.
 *
 * Being sent to the sign-in screen here is a convenience, not a protection. The
 * server refuses an unauthenticated request whatever the browser renders, so a
 * page that loaded without a session would show nothing anyway; this just means
 * somebody sees the sign-in screen rather than an empty one.
 */
export default function AuthenticatedLayout({ children }: { children: ReactNode }) {
  return (
    <SessionProvider>
      <Authenticated>{children}</Authenticated>
    </SessionProvider>
  );
}

function Authenticated({ children }: { children: ReactNode }) {
  const session = useSession();
  const router = useRouter();

  useEffect(() => {
    if (session.status === "signed-out") {
      router.push("/login");
    }
  }, [session.status, router]);

  // Nothing is rendered until the session is known. Rendering the shell first
  // would show navigation built from no capabilities, which is an empty sidebar
  // that fills in a moment later.
  if (session.status === "loading" || session.status === "signed-out") {
    return null;
  }

  if (session.status === "unavailable" || session.user === null) {
    return (
      <main id="main-content" className="app-content">
        <h1>Prepeet is not reachable</h1>
        <p className="lead">
          We could not load your account. This is usually brief. Try again in a moment.
        </p>
      </main>
    );
  }

  return (
    <AppShell
      user={session.user}
      onSignOut={signOut}
      onSwitchTenant={async (tenantId) => {
        await setActiveTenant(tenantId);
        // Re-read rather than assume. Switching changes what the session may do,
        // and the navigation is built from that, so the shell must be rebuilt
        // from what the server says rather than from what was asked for.
        await session.refresh();
      }}
    >
      {children}
    </AppShell>
  );
}
