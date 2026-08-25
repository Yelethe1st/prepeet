import Link from "next/link";

/**
 * Placeholder for the candidate's practice history.
 *
 * The real screen is ported under PRO and SES. This exists so the shell has a
 * destination that is not a dead link, and so signing in lands somewhere rather
 * than on a marketing page. The one real thing it offers is the wizard, which
 * is CAT-04's and already live.
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
      <p className="hint">The session list is ported with SES-07.</p>
    </>
  );
}
