import type { LucideIcon } from "lucide-react";

/**
 * The icon sizes the prototype's `.ic` block defines, in its own values.
 *
 * Stroke width varies with size on purpose: a 14px glyph drawn at the 22px
 * stroke reads as a blob, and a 22px glyph at the 14px stroke reads as a hairline
 * next to the text beside it.
 */
export type IconSize = "sm" | "md" | "lg";

const sizes: Record<IconSize, { px: number; stroke: number }> = {
  sm: { px: 14, stroke: 2 },
  md: { px: 18, stroke: 1.75 },
  lg: { px: 22, stroke: 1.6 },
};

export interface IconProps {
  /** The lucide glyph to draw. The prototype uses lucide, so this is the same set. */
  glyph: LucideIcon;
  size?: IconSize;
  /**
   * A colour utility, when the icon is meant to read differently from the text
   * it sits in. Deliberately not a free `className`: the point of this wrapper
   * is that sizing and the accessibility treatment are not negotiable per call
   * site, and a general className is how they become negotiable.
   */
  tone?: string;
}

/**
 * An icon, and the single place icons are made accessible.
 *
 * Every icon in this product is decoration. WEB-04 recorded that icons stay out
 * of the application until a screen needs one, and the marketing page is that
 * screen: it draws thirty of them, all of them beside the words they illustrate.
 *
 * So `aria-hidden` is applied here rather than asked for at each call site. An
 * icon announced next to the text it repeats is a screen reader saying
 * everything twice, and the one call site that forgets is the one that does it.
 * Where a glyph is the only thing carrying a meaning, the meaning belongs in
 * text beside it, not in a label on the drawing.
 */
export function Icon({ glyph: Glyph, size = "md", tone }: IconProps) {
  const { px, stroke } = sizes[size];

  return (
    <Glyph
      aria-hidden="true"
      focusable="false"
      width={px}
      height={px}
      strokeWidth={stroke}
      className={tone === undefined ? "shrink-0" : `shrink-0 ${tone}`}
    />
  );
}
