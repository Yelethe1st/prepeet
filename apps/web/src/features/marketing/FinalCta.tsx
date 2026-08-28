import { Mic, Send } from "lucide-react";

import { ButtonLink } from "@/shared/components";
import { Icon } from "@/shared/components/Icon";

import { Section } from "./Section";
import { finalCta } from "./content";

/**
 * The closing call to action, ported from the tenth section.
 *
 * The panel is dark in both themes, which the prototype achieves with five
 * literal colours picked to match the dark palette. It sets `data-theme="dark"`
 * on itself instead and then uses ordinary semantic tokens: the custom
 * properties cascade, so inside this panel `--surface` is the same stone the
 * prototype hard-coded and `--fg` is the same near-white, exactly.
 *
 * That is not a shortcut. A literal here would be a colour that exists in one
 * component and nowhere in the design system, and would stay put on the day
 * somebody changes the dark palette.
 */
export function FinalCta() {
  return (
    <Section labelledBy="cta-h" tight>
      <div
        data-theme="dark"
        className="relative overflow-hidden rounded-2xl border border-border bg-surface px-6 py-11 text-center text-fg md:px-10 md:py-16"
      >
        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-0"
          style={{
            background:
              "radial-gradient(ellipse 60% 70% at 50% 120%, color-mix(in srgb, var(--reef-400) 28%, transparent), transparent 70%)",
          }}
        />

        <div className="relative">
          <h2
            id="cta-h"
            className="font-display text-[clamp(1.75rem,3.4vw,2.75rem)] font-medium tracking-[-0.02em]"
          >
            {finalCta.heading}
          </h2>
          <p className="mx-auto mt-3.5 max-w-[520px] leading-relaxed text-fg-2">
            {finalCta.lead}
          </p>

          <div className="mt-7 flex flex-wrap justify-center gap-3">
            <ButtonLink href="/register" size="lg">
              <Icon glyph={Mic} size="sm" />
              Create a free account
            </ButtonLink>
            <ButtonLink href="#how" size="lg" variant="secondary">
              <Icon glyph={Send} size="sm" />
              See how a screen is set up
            </ButtonLink>
          </div>
        </div>
      </div>
    </Section>
  );
}
