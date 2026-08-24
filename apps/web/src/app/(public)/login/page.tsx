"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";

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
      <div className="auth-card">
        <h1>Sign in to Prepeet</h1>
        <p className="lead">
          Practice sessions, screening invitations and your recruiter workspace all live behind one
          account.
        </p>

        <SignInForm
          signIn={async (credentials) => {
            await signIn(credentials);
          }}
          onSignedIn={() => router.push("/practice")}
        />

        <p className="hint" style={{ marginTop: 22 }}>
          New to Prepeet?{" "}
          <Link href="/register" style={{ fontWeight: 600 }}>
            Create an account
          </Link>{" "}
          — free for candidates, no card needed.
        </p>
      </div>
    </AuthShell>
  );
}
