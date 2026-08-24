/**
 * PostCSS, for Tailwind.
 *
 * Tailwind v4 is a PostCSS plugin rather than a config file and a CLI, so the
 * theme lives in CSS at src/shared/styles/theme.css and this is the whole of
 * the build wiring.
 */
const config = {
  plugins: {
    "@tailwindcss/postcss": {},
  },
};

export default config;
