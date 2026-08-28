import { Accessibility, FileCheck2, Lock, ShieldCheck } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { Icon, Wordmark } from "@/shared/components";

/**
 * What the panel promises, and the four things it promises.
 *
 * Kept as data because the prototype shows the same row on both authentication
 * screens, and a row copied into two files is a row that will say two different
 * things by the time anybody notices.
 */
const assurances = [
  { glyph: ShieldCheck, text: "EU data residency (Frankfurt)" },
  { glyph: Lock, text: "Practice data never reaches employers" },
  { glyph: Accessibility, text: "WCAG 2.2 AA" },
  { glyph: FileCheck2, text: "SOC 2 Type II" },
];

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
 *
 * The words are the ones this was built from. A master's student applying for
 * graduate roles, rejected nineteen times with no reason given, hearing his own
 * answer played back and finding the gap in it. That is the whole product in
 * five lines, which is why it sits where somebody signing in will read it.
 *
 * The brand and the footer are here rather than on each screen because the
 * prototype puts them on both, and a header duplicated per screen is a header
 * that goes missing from whichever screen is added last.
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
        className="relative hidden flex-col overflow-hidden bg-stone-900 p-12 text-stone-50 lg:flex"
        aria-label="The story this was built from"
      >
        <p className="text-2xs font-bold tracking-[0.08em] text-reef-300 uppercase">
          The story this was built from
        </p>

        <blockquote className="mt-[18px] max-w-[440px] font-display text-xl leading-[1.35]">
          “Nineteen graduate applications and not one of them told me why. The
          first thing Prepeet played back to me was my own answer on scaling a
          booking service: I named the fix in eleven seconds and never once said
          what I traded away for it. No interviewer had ever told me that, and I
          had been failing on it for a year.”
        </blockquote>

        <p className="mt-4 text-sm text-stone-300">
          Kelvin Onouha · MSc Computer Science · York St John University, London
          <span className="mt-1 block text-stone-400">
            31 practice sessions before his first graduate offer
          </span>
        </p>

        <div className="mt-auto">
          <dl className="grid grid-cols-2 gap-x-6 gap-y-[18px] text-sm text-stone-300">
            <div>
              <dt className="text-2xs font-bold tracking-[0.08em] text-stone-400 uppercase">
                Practice sessions run
              </dt>
              <dd className="mt-1 text-lg font-semibold text-stone-50 tabular-nums">
                184,600
              </dd>
            </div>
            <div>
              <dt className="text-2xs font-bold tracking-[0.08em] text-stone-400 uppercase">
                Employers screening
              </dt>
              <dd className="mt-1 text-lg font-semibold text-stone-50 tabular-nums">
                312
              </dd>
            </div>
          </dl>

          <ul className="mt-8 flex flex-wrap gap-x-[18px] gap-y-2 text-xs text-stone-400">
            {assurances.map((assurance) => (
              <li
                key={assurance.text}
                className="inline-flex items-center gap-1.5"
              >
                <Icon glyph={assurance.glyph} size="sm" />
                {assurance.text}
              </li>
            ))}
          </ul>
        </div>
      </aside>

      <div className="flex flex-col px-6 py-8 sm:px-12">
        <Link
          href="/"
          aria-label="Prepeet home"
          className="flex items-center gap-2.5 self-start text-[18px] font-bold tracking-[-0.02em] text-fg no-underline"
        >
          <Wordmark />
          Prepeet
        </Link>

        <main
          className="flex flex-1 flex-col justify-center py-8"
          id="main-content"
        >
          {children}
        </main>

        <footer className="flex flex-wrap gap-x-4 gap-y-2 text-xs text-fg-3">
          <span>© 2026 Prepeet</span>
          <Link className="text-fg-3 hover:text-fg" href="/">
            About Prepeet
          </Link>
          <Link className="text-fg-3 hover:text-fg" href="/no-workspace">
            Trouble accessing a workspace?
          </Link>
        </footer>
      </div>
    </div>
  );
}
