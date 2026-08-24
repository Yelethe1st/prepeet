/**
 * Placeholder for the candidate's practice history.
 *
 * The real screen is ported under PRO and SES. This exists so the shell has a
 * destination that is not a dead link, and so signing in lands somewhere rather
 * than on a marketing page.
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
      </div>
      <p className="hint">This screen is ported with SES-07.</p>
    </>
  );
}
