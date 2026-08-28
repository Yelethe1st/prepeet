"use client";

import Link from "next/link";

import { ErrorScreen } from "@/features/errors/ErrorScreen";
import { Button } from "@/shared/components";

/**
 * 500, from error-500.html. Next mounts this boundary when rendering throws,
 * so it is the destination for the failures nothing anticipated.
 *
 * The reference is Next's error digest: the same value appears in the server
 * log beside the stack, so quoting it is what turns "it broke" into a
 * findable incident. The prototype also promised that an on-call engineer was
 * already paged; that automation does not exist yet, and this screen claims
 * only what is true — the failure is in the logs under this reference.
 */
export default function ErrorBoundary({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <ErrorScreen
      badge="500 · application error"
      title="Something went wrong on our side"
      actions={
        <>
          <Button type="button" variant="primary" onClick={reset}>
            Retry
          </Button>
          <Link className="text-sm" href="/practice">
            Go to your dashboard
          </Link>
        </>
      }
      facts={
        error.digest
          ? [{ label: "Reference", value: error.digest, mono: true }]
          : undefined
      }
      factsTitle="For support"
    >
      <p>
        This is not something you did, and retrying the same thing may well
        work. We would rather say that plainly than pretend it was a glitch.
      </p>
      {error.digest ? (
        <p>
          The failure was recorded under the reference below. Quote it to
          support and they can see exactly what happened, without you having to
          describe anything.
        </p>
      ) : null}
    </ErrorScreen>
  );
}
