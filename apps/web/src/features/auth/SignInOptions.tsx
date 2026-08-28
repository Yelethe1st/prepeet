import { KeyRound, WandSparkles } from "lucide-react";

import { Icon, TextLink } from "@/shared/components";

/**
 * The other ways into an account, from the list at the foot of
 * screens/login.html.
 *
 * They were four links set in bold body text with no colour, because the
 * prototype's `a { color: var(--primary) }` base rule was never carried across
 * in the port. Four routes into the product read as emphasis, which is the
 * report that started this: the other options were not noticeable enough.
 *
 * A list, with a glyph each, because that is what the prototype draws and
 * because three ways of signing in are a set of choices rather than a sentence.
 *
 * The prototype's fourth entry, "I was invited to a screening interview", is not
 * here: it goes to an invitation screen that is not ported, and a fourth choice
 * that 404s is worse than three that work. It returns with the SCR epic.
 */
export function SignInOptions() {
  return (
    <div className="mt-[22px] border-t border-border pt-[22px]">
      <ul className="flex flex-col gap-2 text-sm">
        <li className="flex items-center gap-2">
          <Icon glyph={KeyRound} size="sm" tone="text-fg-3" />
          <TextLink href="/otp">Sign in with a one-time code</TextLink>
        </li>
        <li className="flex items-center gap-2">
          <Icon glyph={WandSparkles} size="sm" tone="text-fg-3" />
          <TextLink href="/magic-link">
            Email me a sign-in link instead
          </TextLink>
        </li>
      </ul>

      <p className="mt-[22px] text-xs leading-[1.45] text-fg-3">
        New to Prepeet? <TextLink href="/register">Create an account</TextLink>,
        free for candidates, no card needed.
      </p>
    </div>
  );
}
