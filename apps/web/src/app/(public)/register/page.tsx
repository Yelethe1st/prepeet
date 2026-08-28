"use client";

import { TextLink } from "@/shared/components";

import { AuthCard } from "@/features/auth/AuthCard";
import { AuthShell } from "@/features/auth/AuthShell";
import { RegisterForm } from "@/features/auth/RegisterForm";
import { register } from "@/lib/auth/api";

/**
 * The registration route, ported from screens/register.html.
 *
 * There is no navigation on success, deliberately. Registration does not sign
 * anybody in: the address has to be confirmed first, so the form shows what
 * happens next and stays where it is.
 */
export default function RegisterPage() {
  return (
    <AuthShell>
      <AuthCard
        title="Create your Prepeet account"
        lead="Practise interviews for yourself, or set up a workspace to screen candidates for your organisation."
      >
        <RegisterForm register={register} onRegistered={() => {}} />

        <p className="mt-[22px] text-xs leading-[1.45] text-fg-3">
          Already have an account? <TextLink href="/login">Sign in</TextLink>.
        </p>
      </AuthCard>
    </AuthShell>
  );
}
