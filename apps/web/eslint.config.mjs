// ESLint configuration for the web application.
//
// This file exists because `pnpm lint` ran `next lint`, which Next 16 removed,
// and because there was no ESLint configuration anywhere in the repository. The
// script had never worked. Nothing noticed, because CI ran `pnpm typecheck` and
// `pnpm test:coverage` for this package and never `pnpm lint`, so the one place
// that would have reported it was not looking.
//
// Both halves are fixed together: this makes the command work, and CI now
// invokes the same make targets an engineer runs, so the two cannot diverge
// again by one of them quietly omitting a step.
import next from 'eslint-config-next'
import coreWebVitals from 'eslint-config-next/core-web-vitals'
import typescript from 'eslint-config-next/typescript'

const config = [
  {
    // Generated and build output. Linting generated code reports problems
    // nobody can fix in the file where they appear, which teaches people to
    // ignore the linter.
    ignores: [
      '.next/**',
      'coverage/**',
      'node_modules/**',
      'next-env.d.ts',
    ],
  },
  ...next,
  ...coreWebVitals,
  ...typescript,
]

export default config
