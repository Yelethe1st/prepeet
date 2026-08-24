"use client";

import Link from "next/link";

import { AuthShell } from "@/features/auth/AuthShell";
import { RegisterForm } from "@/features/auth/RegisterForm";
import { register } from "@/features/auth/api";

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
      <div className="auth-card">
        <h1>Create your Prepeet account</h1>
        <p className="lead">
          Practise interviews for yourself, or set up a workspace to screen candidates for your
          organisation.
        </p>

        <RegisterForm register={register} onRegistered={() => {}} />

        <p className="hint" style={{ marginTop: 22 }}>
          Already have an account?{" "}
          <Link href="/login" style={{ fontWeight: 600 }}>
            Sign in
          </Link>
          .
        </p>
      </div>
    </AuthShell>
  );
}
