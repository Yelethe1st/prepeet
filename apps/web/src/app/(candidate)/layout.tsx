"use client";

import type { ReactNode } from "react";

import "@/shared/styles/theme.css";
import { useRouter } from "next/navigation";
import { useEffect } from "react";

import { useQueryClient } from "@tanstack/react-query";

import {
  SessionProvider,
  sessionQueryKey,
  useSession,
} from "@/lib/auth/session";
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
export default function AuthenticatedLayout({
  children,
}: {
  children: ReactNode;
}) {
  return (
    <SessionProvider>
      <Authenticated>{children}</Authenticated>
    </SessionProvider>
  );
}

function Authenticated({ children }: { children: ReactNode }) {
  const session = useSession();
  const router = useRouter();
  const client = useQueryClient();

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
          We could not load your account. This is usually brief. Try again in a
          moment.
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
        // Everything cached belonged to the workspace being left.
        //
        // IAM-03's last criterion is that switching cannot expose a resource
        // from the previous tenant, including through a cached read model, and
        // this is that cache. Every query key in the application is scoped by
        // what it reads rather than by whose it is: ["sessions"], ["profile"],
        // ["documents"] are the same key in both workspaces, and the client
        // answers from cache before it revalidates, so the first paint after a
        // switch was the previous tenant's data.
        //
        // Removed by exclusion rather than by listing what to drop. Adding the
        // tenant to every key would work until somebody adds a key and
        // forgets, and the forgotten one is the leak; naming what to remove
        // has the same flaw. Everything goes except the session itself, which
        // is what says who the caller now is and is re-read on the next line.
        client.removeQueries({
          predicate: (query) => query.queryKey[0] !== sessionQueryKey[0],
        });
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
