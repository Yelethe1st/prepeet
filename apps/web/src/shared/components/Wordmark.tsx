/**
 * The brand mark: four bars of a waveform in a rounded square.
 *
 * Ported from the SVG in screens/assets/js/shell.js, which every prototype
 * screen injects into its header. The application had been drawing an empty
 * rounded square in its place, which is the mark with the mark missing.
 *
 * `currentColor` on the bars, so the square carries the colour and the glyph
 * follows it. Hidden from assistive technology: the word "Prepeet" is always
 * beside it, and a logo announced next to its own wordmark is the name twice.
 */
export function Wordmark({ size = 30 }: { size?: number }) {
  return (
    <span
      aria-hidden="true"
      className="grid flex-none place-items-center rounded-[9px] bg-primary text-primary-fg"
      style={{ width: size, height: size }}
    >
      <svg
        viewBox="0 0 24 24"
        fill="none"
        width={size * 0.6}
        height={size * 0.6}
      >
        <rect
          x="3"
          y="9.5"
          width="2.6"
          height="5"
          rx="1.3"
          fill="currentColor"
        />
        <rect
          x="8"
          y="5"
          width="2.6"
          height="14"
          rx="1.3"
          fill="currentColor"
        />
        <rect
          x="13"
          y="7.5"
          width="2.6"
          height="9"
          rx="1.3"
          fill="currentColor"
        />
        <rect
          x="18"
          y="10.5"
          width="2.6"
          height="3"
          rx="1.3"
          fill="currentColor"
        />
      </svg>
    </span>
  );
}
