"use client";

import { usePathname } from "next/navigation";
import { useEffect } from "react";

import { configureTracing, startClientTrace } from "./tracing";

/**
 * Starts one trace per navigation, so the calls a screen makes are one wait.
 *
 * A navigation rather than a request. A screen that loads a profile and a
 * session list made one gesture, and splitting it into two traces means nobody
 * can see the wait the person actually experienced.
 *
 * Mounted once in the root layout rather than per screen, because a screen that
 * forgot to mount it would go untraced and look identical to one that was
 * simply fast. Correlation belongs to the page, not to whoever writes the next
 * feature.
 *
 * Off unless `NEXT_PUBLIC_TRACING` is `true`. Reading the environment here
 * rather than in the tracing module keeps that module testable without
 * process.env, and keeps a server-rendered pass from picking up a browser
 * setting.
 */
export function ClientTracing() {
  const pathname = usePathname();

  useEffect(() => {
    configureTracing({ enabled: process.env.NEXT_PUBLIC_TRACING === "true" });
  }, []);

  useEffect(() => {
    startClientTrace();
  }, [pathname]);

  return null;
}
