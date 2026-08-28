import type { ReactNode } from "react";

/**
 * The card every authentication screen puts its content in.
 *
 * This exists because its predecessor did not: login and register used an
 * `auth-card` class that survived the Tailwind conversion as a name with no
 * definition, so the card rendered unstyled — full width, unspaced — and the
 * regenerated visual baselines quietly accepted that as the look. Tailwind
 * emits nothing for a class it does not recognise, and nothing warns.
 *
 * The values are the prototype's `.auth-card`, `.auth-card h1` and `.lead`
 * rules, translated to utilities over the mapped tokens.
 */
export function AuthCard({
  title,
  lead,
  children,
}: {
  title: string;
  lead?: string;
  children: ReactNode;
}) {
  return (
    <div className="mx-auto w-full max-w-[420px] py-8">
      <h1 className="font-display text-2xl tracking-[-0.02em]">{title}</h1>
      {lead ? (
        <p className="mt-1.5 mb-6 text-sm leading-[1.55] text-fg-2">{lead}</p>
      ) : null}
      {children}
    </div>
  );
}
