import Link from "next/link";
import { Suspense } from "react";

import { SessionsScreen } from "@/features/sessions/SessionsScreen";

/**
 * The candidate's practice home: the whole session history with the way
 * onward from every state, and the wizard for the next one. Suspense wraps
 * the screen because the filter lives in the URL's search params.
 */
export default function PracticePage() {
  return (
    <>
      <div className="page-header">
        <div>
          <h1>Practice</h1>
          <p className="page-desc">
            Your practice sessions live here. Nothing an employer can see, ever.
          </p>
        </div>
        <Link
          href="/practice/new"
          className="inline-flex items-center rounded-md bg-primary px-4 py-2 text-sm font-semibold text-primary-fg"
        >
          Start a practice interview
        </Link>
      </div>
      <section aria-label="Session history" className="mt-6">
        <Suspense>
          <SessionsScreen />
        </Suspense>
      </section>
    </>
  );
}
