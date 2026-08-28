import {
  CircleDot,
  ClipboardCheck,
  Layers,
  Mic,
  Timer,
  Volume2,
} from "lucide-react";

import { ButtonLink } from "@/shared/components";
import { Icon } from "@/shared/components/Icon";

import { hero } from "./content";

/**
 * The bar heights in the still of the waveform.
 *
 * Deterministic, and deliberately not random. The prototype animates this and
 * the animation is decoration; a still of it is what a marketing page needs.
 * Random heights would differ between the server's render and the browser's,
 * which React reports as a hydration mismatch, and would make the visual
 * baseline differ from itself on every run.
 */
const bars = Array.from(
  { length: 26 },
  (_, index) => 6 + Math.round(28 * Math.abs(Math.sin(index * 0.7))),
);

/**
 * The hero, ported from the `.mk-hero` block.
 *
 * The picture beside the copy is the live interview screen, and it is a picture:
 * `role="img"` with a description, everything inside it hidden from assistive
 * technology. It is a drawing of a product, not the product, and announcing its
 * parts would read as a session somebody is in.
 *
 * One recorded deviation. The prototype's second call to action goes to the
 * recruiter dashboard, which is not ported. It points at the section of this
 * page that shows exactly what a recruiter sees instead, which is the same
 * promise kept with a destination that exists.
 */
export function Hero() {
  return (
    <section aria-labelledby="hero-h" className="relative overflow-hidden">
      {/*
        The glow behind the hero. A real element rather than a pseudo-element so
        it is written in the same place as everything else it sits behind, and
        hidden because it is a wash of colour with nothing to say.
      */}
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0"
        style={{
          background:
            "radial-gradient(ellipse 50% 45% at 70% 20%, color-mix(in srgb, var(--primary) 14%, transparent), transparent 70%)",
        }}
      />

      <div className="relative mx-auto grid max-w-[1180px] grid-cols-1 items-center gap-10 px-5 pt-14 pb-10 lg:grid-cols-[minmax(0,1.05fr)_minmax(0,1fr)] lg:gap-14 lg:px-6 lg:pt-22 lg:pb-16">
        <div>
          <p className="text-2xs font-bold tracking-[0.1em] text-primary uppercase">
            {hero.eyebrow}
          </p>

          <h1
            id="hero-h"
            className="mt-3 font-display text-[clamp(2.5rem,5.2vw,4.25rem)] leading-[1.05] font-medium tracking-[-0.025em]"
          >
            {hero.headingBefore}
            <em className="text-primary italic">{hero.headingEmphasis}</em>
            {hero.headingAfter}
          </h1>

          <p className="mt-5 max-w-[540px] text-md leading-relaxed text-fg-2">
            {hero.leadBefore}
            <strong className="font-semibold text-fg">
              {hero.leadEmphasis}
            </strong>
            {hero.leadAfter}
          </p>

          <div className="mt-8 flex flex-wrap gap-3">
            <ButtonLink href="/register" size="lg">
              <Icon glyph={Mic} size="sm" />
              Start practising
            </ButtonLink>
            <ButtonLink href="#how" size="lg" variant="secondary">
              <Icon glyph={ClipboardCheck} size="sm" />
              See what a recruiter sees
            </ButtonLink>
          </div>

          <ul className="mt-4 flex flex-wrap gap-x-3.5 gap-y-2 text-xs text-fg-3">
            {hero.assurances.map((assurance) => (
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

        <div
          role="img"
          aria-label={hero.frameDescription}
          className="overflow-hidden rounded-xl border border-border bg-surface shadow-lg"
        >
          <div className="flex h-10 items-center gap-2 border-b border-border bg-surface-2 px-3.5 text-xs text-fg-3">
            <span className="size-2.5 rounded-full bg-border-strong" />
            <span className="size-2.5 rounded-full bg-border-strong" />
            <span className="size-2.5 rounded-full bg-border-strong" />
            <span className="ml-1.5 truncate font-mono">{hero.framePath}</span>
          </div>

          <div aria-hidden="true" className="p-5">
            <div className="mb-4 flex items-center justify-between">
              <span className="inline-flex items-center gap-2 text-sm font-semibold">
                <Icon glyph={Layers} size="sm" tone="text-primary" />
                {hero.framePhase}
              </span>
              <span className="inline-flex items-center gap-1.5 font-mono text-sm font-semibold">
                <Icon glyph={Timer} size="sm" />
                {hero.frameTimer}
              </span>
            </div>

            <div className="mb-3.5 flex flex-wrap justify-center gap-2">
              <span className="inline-flex items-center rounded-pill bg-primary-soft px-2.5 py-1 font-mono text-2xs tracking-[0.08em] text-primary-soft-fg uppercase">
                Practice
              </span>
              <span className="inline-flex items-center gap-1.5 rounded-pill border border-border-strong px-2.5 py-1 text-2xs text-fg-2">
                <span className="size-1.5 rounded-full bg-success" />
                Connected
              </span>
            </div>

            <div className="flex flex-col items-center gap-3.5">
              {/*
                The persona orb: a ring around the interviewer's avatar, which
                is how the live screen shows who is speaking. The avatar takes
                the interviewer's own colour rather than the brand's, so the
                two speakers in a transcript stay told apart by it.
              */}
              <span className="grid size-24 place-items-center rounded-full border border-border-strong bg-surface-2">
                {/*
                  The initials take the ordinary foreground rather than the
                  interviewer's own colour. That colour on a tint of itself is
                  the shade the prototype uses and it does not reach 4.5:1 in
                  the dark theme, which the contrast sweep caught.
                */}
                <span className="grid size-[62px] place-items-center rounded-full bg-waveform-ai/25 text-md font-semibold text-fg">
                  RA
                </span>
              </span>
              <span className="inline-flex items-center gap-1.5 rounded-pill bg-surface-3 px-3 py-1.5 text-xs font-semibold text-fg-2">
                <Icon glyph={Volume2} size="sm" />
                {hero.frameSpeaker}
              </span>
              <div className="flex h-11 items-center justify-center gap-[3px]">
                {bars.map((height, index) => (
                  <span
                    key={index}
                    className="w-1 rounded-sm bg-waveform-ai"
                    style={{ height }}
                  />
                ))}
              </div>
            </div>

            <div className="mt-4 rounded-md border border-border bg-surface-2 p-3.5 text-sm leading-normal">
              <span className="mb-1 block text-2xs font-semibold text-fg-3 uppercase">
                {hero.frameCaptionWho}
              </span>
              {hero.frameCaption}
            </div>

            <div className="mt-4 flex items-center justify-between gap-3">
              <span className="flex gap-1">
                {[0, 1, 2, 3, 4, 5].map((step) => (
                  <span
                    key={step}
                    className={
                      "h-1 w-5 rounded-pill " +
                      (step < 3
                        ? "bg-primary"
                        : step === 3
                          ? "bg-primary-border"
                          : "bg-border")
                    }
                  />
                ))}
              </span>
              <span className="inline-flex items-center gap-1.5 text-xs text-fg-3">
                <Icon glyph={CircleDot} size="sm" />
                {hero.frameHint}
              </span>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
