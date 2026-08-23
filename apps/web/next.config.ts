import type { NextConfig } from "next";

/**
 * Next.js configuration.
 *
 * `reactStrictMode` stays on because the realtime surfaces in RTC-06 depend on
 * effects being cleanly torn down, and strict mode surfaces a missing cleanup
 * during development rather than as a stuck microphone in production.
 */
const nextConfig: NextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,
};

export default nextConfig;
