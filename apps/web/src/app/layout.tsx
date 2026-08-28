import type { Metadata } from "next";
import { Figtree, Fraunces, JetBrains_Mono } from "next/font/google";
import type { ReactNode } from "react";

import "@/shared/styles/theme.css";

import { DEFAULT_THEME, themeScript } from "@/shared/themePreference";
import { QueryProvider } from "@/lib/api/QueryProvider";

/*
 * The three faces the design system names.
 *
 * tokens.css has asked for Figtree, Fraunces and JetBrains Mono since WEB-01
 * and nothing ever fetched them, so every screen rendered in the fallbacks:
 * system-ui for the body and Georgia for the display face. A token that names a
 * font nobody loaded is a token that is not telling the truth, and the display
 * face is most of the difference between the prototype's front page and a page
 * that merely has the same words on it.
 *
 * Served by the application rather than by Google: next/font downloads the
 * files at build time and serves them from this origin, so there is no request
 * to a third party from a visitor's browser and nothing to consent to.
 *
 * No `weight`. All three are variable fonts, so one file covers every weight the
 * design system uses, and listing weights would download a static instance per
 * weight instead.
 *
 * `display: "swap"` so text is readable in the fallback while the face loads.
 * The alternative is invisible text on a slow connection, which is worse than
 * text in the wrong font.
 */
const sans = Figtree({
  subsets: ["latin"],
  variable: "--font-figtree",
  display: "swap",
});
const display = Fraunces({
  subsets: ["latin"],
  variable: "--font-fraunces",
  display: "swap",
});
const mono = JetBrains_Mono({
  subsets: ["latin"],
  variable: "--font-jetbrains",
  display: "swap",
});

export const metadata: Metadata = {
  title: "Prepeet",
  description:
    "Voice-first interview practice and structured employer screening.",
};

/**
 * The document shell.
 *
 * `lang` is set explicitly because assistive technology chooses a voice from it,
 * and the product is en-GB first.
 *
 * The theme attribute is written twice on purpose. It is rendered here so the
 * server-rendered markup already carries the default, and then corrected by the
 * inline script before the page paints if the person has chosen otherwise.
 * Without the script there is a flash of the default theme on every load for
 * anybody who chose the other one; without the attribute the server renders a
 * document with no theme at all, which the script then fixes but which is what
 * a client with scripting disabled is left with.
 */
export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html
      lang="en-GB"
      data-theme={DEFAULT_THEME}
      // The three font variables land on the document element, which is where
      // tokens.css reads them from.
      className={`${sans.variable} ${display.variable} ${mono.variable}`}
      suppressHydrationWarning
    >
      <head>
        {/*
          Runs before anything paints. suppressHydrationWarning is on the html
          element rather than here: the script changes the attribute React
          rendered, which is exactly its job, and React would otherwise report
          the difference as a hydration mismatch.
        */}
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body>
        {/*
          At the root so every route shares one cache. Two providers would mean
          two caches and the same request made twice, which is the problem the
          library is there to solve.
        */}
        <QueryProvider>{children}</QueryProvider>
      </body>
    </html>
  );
}
