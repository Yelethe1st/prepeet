"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";

import { AuthCard } from "@/features/auth/AuthCard";
import { AuthShell } from "@/features/auth/AuthShell";
import { SignInForm } from "@/features/auth/SignInForm";
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

        <div className="mt-[22px] space-y-1.5 text-xs leading-[1.45] text-fg-3">
          <p>
            <Link className="font-semibold" href="/forgot-password">
              Forgot your password?
            </Link>
          </p>
          <p>
            Prefer not to type it?{" "}
            <Link className="font-semibold" href="/magic-link">
              Email me a sign-in link
            </Link>{" "}
            or{" "}
            <Link className="font-semibold" href="/otp">
              a one-time code
            </Link>
            .
          </p>
          <p>
            New to Prepeet?{" "}
            <Link className="font-semibold" href="/register">
              Create an account
            </Link>{" "}
            — free for candidates, no card needed.
          </p>
        </div>
      </AuthCard>
    </AuthShell>
  );
}
