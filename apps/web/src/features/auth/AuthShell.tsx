import type { ReactNode } from "react";

/**
 * The two-panel layout both authentication screens use.
 *
 * Ported from screens/login.html and screens/register.html, which share it. The
 * dark side panel is hidden below 1024px by the ported layout.css, so on a
 * phone this is the form and nothing else.
 *
 * The side panel carries a candidate's words rather than product copy, which is
 * the prototype's choice and worth preserving: it is the one place in the
 * product that says what the thing is for in somebody's own voice.
 */
export function AuthShell({ children }: { children: ReactNode }) {
  return (
    <>
      <a className="skip-link" href="#main-content">
        Skip to main content
      </a>

      <aside className="auth-side" aria-label="Why people practise with Prepeet">
        <p className="eyebrow">Why people practise here</p>
        <blockquote className="quote" style={{ margin: "18px 0 0" }}>
          “Eleven rejections and nobody would tell me why. The first thing Prepeet played back to me
          was my own answer about a deteriorating patient — I described the fix in nine seconds and
          never once said how I knew. I have not answered a question that way since.”
        </blockquote>
        <p className="quote-by">
          Amara Osei · Registered Nurse, Intensive Care · Manchester
        </p>

        <div className="side-foot">
          <div className="trust">
            <span>UK data residency</span>
            <span>Practice data never reaches employers</span>
            <span>WCAG 2.2 AA</span>
          </div>
        </div>
      </aside>

      <main className="auth-main" id="main-content">
        {children}
      </main>
    </>
  );
}
