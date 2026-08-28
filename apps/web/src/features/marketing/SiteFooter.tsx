import Link from "next/link";

import { ThemeToggle, Wordmark } from "@/shared/components";

import { footer } from "./content";

/**
 * The site footer, ported from `.mk-footer`.
 *
 * Each column is its own `nav` with its own heading, which is what makes them
 * separately navigable rather than one list of thirty links. The prototype does
 * the same, and its column headings are `h3` so the document's heading levels
 * stay contiguous after the `h2` sections above.
 *
 * Two recorded deviations. The employer column linked to six recruiter screens,
 * none of which is ported, so it answers with the sections of this page that
 * cover the same ground. The three legal documents are issued at contract and
 * have no page to link to, so they are stated rather than linked, which is what
 * the prototype does with them too.
 */
export function SiteFooter() {
  return (
    <footer className="border-t border-border bg-surface-2">
      <div className="mx-auto grid max-w-[1180px] grid-cols-1 gap-8 px-5 pt-12 pb-7 sm:grid-cols-2 md:px-6 lg:grid-cols-[1.5fr_repeat(4,minmax(0,1fr))]">
        <div>
          <Link
            href="/"
            aria-label="Prepeet home"
            className="flex items-center gap-2.5 text-[18px] font-bold tracking-[-0.02em] text-fg no-underline"
          >
            <Wordmark />
            Prepeet
          </Link>
          <p className="mt-3 max-w-[280px] text-xs text-fg-3">{footer.blurb}</p>
          <div className="mt-4">
            <ThemeToggle withLabel />
          </div>
        </div>

        {footer.columns.map((column) => (
          <nav key={column.id} aria-labelledby={column.id}>
            <h3
              id={column.id}
              className="mb-3 text-xs font-semibold tracking-[0.1em] text-fg-3 uppercase"
            >
              {column.heading}
            </h3>
            <ul className="flex flex-col gap-2">
              {column.links.map((link) => (
                <li key={`${column.id}-${link.label}`}>
                  <a
                    href={link.href}
                    className="text-sm text-fg-2 no-underline hover:text-fg"
                  >
                    {link.label}
                  </a>
                </li>
              ))}
              {column.id === "foot-company"
                ? footer.notices.map((notice) => (
                    <li key={notice} className="text-sm text-fg-3">
                      {notice}
                    </li>
                  ))
                : null}
            </ul>
          </nav>
        ))}
      </div>

      <div className="mx-auto flex max-w-[1180px] flex-wrap justify-between gap-3 border-t border-border px-5 pt-4.5 pb-7 text-xs text-fg-3 md:px-6">
        <span>{footer.copyright}</span>
        <span>{footer.disclaimer}</span>
      </div>
    </footer>
  );
}
