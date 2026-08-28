/**
 * `next/font/google`, for the test environment only.
 *
 * The real module is not a module in any ordinary sense: it is a marker the
 * Next compiler rewrites at build time, downloading the font files and emitting
 * a class and a CSS variable in its place. Nothing performs that rewrite under
 * Vitest, so importing it gives back an object whose properties are not
 * functions, and calling one fails with "Figtree is not a function" from the
 * root layout, three tests away from anything about fonts.
 *
 * Aliased here rather than mocked per file, because the failure is a property of
 * the environment and not of any one test: the next component to use a font
 * would meet exactly the same error and have to discover the cause again.
 *
 * It returns the shape the real one returns, with the variable name the caller
 * asked for. What it cannot do is prove a font loads, which is a build-time and
 * then a rendering question, and the browser suite is where it is answered: the
 * visual baselines are of the real faces.
 */
interface FontOptions {
  variable?: string;
  subsets?: string[];
  weight?: string | string[];
  display?: string;
}

interface LoadedFont {
  className: string;
  variable: string;
  style: { fontFamily: string };
}

function face(family: string): (options?: FontOptions) => LoadedFont {
  return (options: FontOptions = {}) => ({
    className: `font-${family}`,
    variable: options.variable ?? `--font-${family}`,
    style: { fontFamily: family },
  });
}

export const Figtree = face("figtree");
export const Fraunces = face("fraunces");
export const JetBrains_Mono = face("jetbrains-mono");
