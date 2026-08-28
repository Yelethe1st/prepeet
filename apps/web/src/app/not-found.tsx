"use client";

import { usePathname } from "next/navigation";

import { ErrorScreen } from "@/features/errors/ErrorScreen";
import { ButtonLink, TextLink } from "@/shared/components";

/**
 * 404, from error-404.html. Next renders this for any route that does not
 * exist, which is what makes it a destination rather than a page somebody
 * has to remember to link.
 *
 * The prototype's screen-index search did not port: there is no screen index
 * yet, and a search box wired to nothing is worse than the two destinations
 * that do exist. It arrives with the screens it would search.
 */
export default function NotFound() {
  const path = usePathname();

  return (
    <ErrorScreen
      badge="404 · not found"
      title="We could not find that page"
      actions={
        <>
          <ButtonLink href="/practice" variant="primary">
            Go to your dashboard
          </ButtonLink>
          <TextLink href="/login">Sign in</TextLink>
        </>
      }
      facts={[{ label: "Requested", value: path ?? "", mono: true }]}
      factsTitle="What was requested"
    >
      <p>
        There is nothing at this address. It may have been renamed, the thing it
        pointed at may have been deleted, or the link may simply have a typo in
        it.
      </p>
    </ErrorScreen>
  );
}
