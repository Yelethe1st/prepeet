import Link from "next/link";
import type { ReactNode } from "react";

/**
 * A link inside a sentence.
 *
 * It exists because the prototype's base stylesheet colours every anchor with
 * `a { color: var(--primary) }` and that rule was never carried across. Every
 * inline link in the product had been rendering as bold body text: the sign-in
 * screen's "Email me a sign-in link", "a one-time code", "Forgot your password?"
 * and "Create an account" all looked like emphasis rather than like the four
 * other ways into the account that they are.
 *
 * Colour and an underline, not colour alone. WCAG 1.4.1 is explicit that a link
 * in a block of text cannot be distinguished by colour on its own, and this is
 * exactly that case: four links in three sentences of small print. The underline
 * is drawn faint and firms up on hover, which is the prototype's hover
 * behaviour arrived at from the other direction.
 *
 * It sets no size. The size belongs to the sentence the link sits in, and a
 * component that decided it would be a component fighting its own container.
 */
export function TextLink({
  href,
  children,
}: {
  href: string;
  children: ReactNode;
}) {
  return (
    <Link
      href={href}
      className="font-semibold text-primary underline decoration-primary/40 underline-offset-2 hover:decoration-primary"
    >
      {children}
    </Link>
  );
}
