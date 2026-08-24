import type { ReactNode } from "react";

/**
 * The two-panel layout both authentication screens use.
 *
 * Ported from screens/login.html and screens/register.html, which share it. The
 * prototype expressed it as `.auth`, `.auth-side` and `.auth-main` in
 * layout.css; here it is the same arrangement in utilities, so the grid, the
 * breakpoint and the spacing are still the prototype's values by way of the
 * tokens Tailwind is configured from.
 *
 * The side panel is hidden below `lg`, exactly as the prototype hides it below
 * 1024px. On a phone this is the form and nothing else, which matters more than
 * it sounds: if the panel were shown the form would be pushed off screen and
 * somebody could not sign in at all.
 *
 * The panel carries a candidate's words rather than product copy. That is the
 * prototype's choice and worth preserving: it is the one place in the product
 * that says what the thing is for in somebody's own voice.
 */
export function AuthShell({ children }: { children: ReactNode }) {
  return (
    <div className="grid min-h-screen grid-cols-1 lg:grid-cols-2">
      <a className="skip-link" href="#main-content">
        Skip to main content
      </a>

      <aside
        // Painted from the palette rather than from semantic tokens, because
        // this panel is dark in both themes. `bg-fg` inverts with the theme,
        // which made it light-on-light in dark mode: caught by the browser
        // suite's contrast check, and invisible to everything else.
        className="relative hidden overflow-hidden bg-stone-900 p-12 text-stone-50 lg:block"
        aria-label="Why people practise with Prepeet"
      >
        <p className="text-2xs font-bold tracking-[0.08em] text-reef-300 uppercase">
          Why people practise here
        </p>

        <blockquote className="mt-[18px] max-w-[440px] font-display text-xl leading-[1.35]">
          “Eleven rejections and nobody would tell me why. The first thing Prepeet played back to me
          was my own answer about a deteriorating patient — I described the fix in nine seconds and
          never once said how I knew. I have not answered a question that way since.”
        </blockquote>

        <p className="mt-4 text-sm text-stone-300">
          Amara Osei · Registered Nurse, Intensive Care · Manchester
        </p>

        <div className="absolute inset-x-12 bottom-10 flex flex-wrap gap-[18px] text-xs text-stone-400">
          <span>UK data residency</span>
          <span>Practice data never reaches employers</span>
          <span>WCAG 2.2 AA</span>
        </div>
      </aside>

      <main className="flex flex-col justify-center px-6 py-12 sm:px-12" id="main-content">
        {children}
      </main>
    </div>
  );
}
