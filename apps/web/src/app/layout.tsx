import type { Metadata } from "next";
import type { ReactNode } from "react";

import "@/design-system/tokens.css";
import { DEFAULT_THEME, themeScript } from "@/design-system/themePreference";

export const metadata: Metadata = {
  title: "Prepeet",
  description: "Voice-first interview practice and structured employer screening.",
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
    <html lang="en-GB" data-theme={DEFAULT_THEME} suppressHydrationWarning>
      <head>
        {/*
          Runs before anything paints. suppressHydrationWarning is on the html
          element rather than here: the script changes the attribute React
          rendered, which is exactly its job, and React would otherwise report
          the difference as a hydration mismatch.
        */}
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body>{children}</body>
    </html>
  );
}
