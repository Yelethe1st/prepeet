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

  /**
   * The API is reached on the same origin as the page, and this is what makes
   * that true while developing.
   *
   * The contract declares its server as `/api/v1`, a path rather than an
   * absolute URL, which is a statement that the browser and the API share an
   * origin. In a deployed environment the load balancer arranges that. Locally
   * the two are separate processes on separate ports, so without this rewrite
   * the browser would ask Next for `/api/v1/auth/login` and get a 404.
   *
   * Same-origin rather than pointing the client at http://localhost:8080 for
   * two reasons beyond matching the contract. It needs no CORS configuration,
   * so the local arrangement does not differ from the deployed one in a way
   * that hides a missing header until staging. And the session cookie is
   * SameSite=Lax: it would survive this particular cross-origin case, since
   * ports do not make two URLs cross-site, but relying on that means the first
   * time the API moves to its own hostname the cookies stop being sent and
   * nothing explains why.
   */
  async rewrites() {
    const api = process.env.PREPEET_API_ORIGIN ?? "http://localhost:8080";
    return [{ source: "/api/:path*", destination: `${api}/api/:path*` }];
  },
};

export default nextConfig;
