"use client";

import { useRouter } from "next/navigation";

import { AuthCard } from "@/features/auth/AuthCard";
import { AuthShell } from "@/features/auth/AuthShell";
import { SignInForm } from "@/features/auth/SignInForm";
import { SignInOptions } from "@/features/auth/SignInOptions";
import { signIn } from "@/lib/auth/api";

/**
 * The sign-in route, ported from screens/login.html.
 *
 * The page owns routing and nothing else. Everything a person interacts with is
 * in SignInForm, which is why that has tests and this does not need them.
 *
 * The prototype's post-sign-in destination handling is not here yet. It has a
 * server-side counterpart in api.SafeRedirect, which validates rather than
 * sanitises a `next` parameter, and wiring the two together belongs with the
 * screens that produce such links.
 */
export default function LoginPage() {
  const router = useRouter();

  return (
    <AuthShell>
      <AuthCard
        title="Sign in to Prepeet"
        lead="Practice sessions, screening invitations and your recruiter workspace all live behind one account."
      >
        <SignInForm
          signIn={async (credentials) => {
            await signIn(credentials);
          }}
          onSignedIn={() => router.push("/practice")}
        />
        <SignInOptions />
      </AuthCard>
    </AuthShell>
  );
}
