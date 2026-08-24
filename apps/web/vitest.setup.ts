import * as axeMatchers from "vitest-axe/matchers";
import { expect } from "vitest";

import "@testing-library/jest-dom/vitest";

/**
 * Test setup for the web application.
 *
 * The axe matchers are registered here rather than per file. vitest-axe was in
 * the dependencies and its matchers had never been extended onto expect, so the
 * first `toHaveNoViolations` written failed with "Invalid Chai property" rather
 * than with an accessibility result. A harness that makes the accessible path
 * the awkward one gets the accessibility it deserves, and PLT-10 asks for the
 * opposite.
 */
expect.extend(axeMatchers);
