import type { Metadata } from "next";
import type { ReactNode } from "react";

import "@/design-system/tokens.css";

export const metadata: Metadata = {
  title: "Prepeet",
  description: "Voice-first interview practice and structured employer screening.",
};

/**
 * The document shell.
 *
 * `lang` is set explicitly because assistive technology chooses a voice from
 * it, and the product is en-GB first. Theme is resolved on the client by the
 * theme module ported in WEB-01, so no theme attribute is set here.
 */
export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en-GB">
      <body>{children}</body>
    </html>
  );
}
