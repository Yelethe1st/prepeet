"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import type { ReactNode } from "react";

import { ApiError } from "./client";

/**
 * The query client, for everything that reads from the API.
 *
 * Created in state rather than at module scope. A module-level client is shared
 * by every request the server renders, which on a server means one person's
 * cached data can be handed to the next; in state it is per browser, which is
 * the only correct scope for anything derived from a session.
 */
export function QueryProvider({ children }: { children: ReactNode }) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            /*
             * Nothing is retried after a refusal.
             *
             * 401, 403 and 404 are answers rather than failures, and retrying
             * one is three more requests that will be refused for the same
             * reason. It also delays the screen that explains what happened.
             */
            retry: (attempt, error) => {
              if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
                return false;
              }
              return attempt < 2;
            },
            // Data is considered current for a moment, so two components
            // mounting together make one request rather than two.
            staleTime: 5_000,
            refetchOnWindowFocus: false,
          },
        },
      }),
  );

  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
