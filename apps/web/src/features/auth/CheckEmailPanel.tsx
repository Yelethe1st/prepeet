"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

import { ApiError } from "@/lib/api/client";
import type { TokenEmailKind } from "@/lib/auth/api";
import { Banner, Button } from "@/shared/components";

import { maskEmail } from "./sentEmail";

/**
 * The screen after an email goes out: what was sent, where, and the resend.
 *
 * The resend cooldown is the visible one IAM-02 requires. Its length comes
 * from the server's 429 rather than from a constant here, so the countdown a
 * person watches is the cooldown that actually holds; a fresh send starts a
 * local estimate that the next 429, if any, corrects.
 */

/** What each flow's email is, in the words the prototype uses. */
const described: Record<TokenEmailKind, { subject: string; validity: string }> =
  {
    password_reset: {
      subject: "Set a new Prepeet password",
      validity: "30 minutes, and one use only",
    },
    magic_link: {
      subject: "Your sign-in link",
      validity: "15 minutes, and one use only",
    },
    otp: {
      subject: "Your verification code",
      validity: "10 minutes, and one use only",
    },
    verify_email: {
      subject: "Verify your email address",
      validity: "30 minutes, and one use only",
    },
  };

/** The cooldown assumed after a successful send, until a 429 says otherwise. */
const assumedCooldownSeconds = 60;

interface CheckEmailPanelProps {
  kind: TokenEmailKind;
  /** The address, or empty when storage was unavailable. */
  email: string;
  request: (kind: TokenEmailKind, email: string) => Promise<void>;
  /** Where "wrong address?" goes: back to the form that asked. */
  changeAddressHref: string;
}

export function CheckEmailPanel({
  kind,
  email,
  request,
  changeAddressHref,
}: CheckEmailPanelProps) {
  const [cooldown, setCooldown] = useState(assumedCooldownSeconds);
  const [failure, setFailure] = useState<string | null>(null);
  const [resent, setResent] = useState(false);

  useEffect(() => {
    if (cooldown <= 0) return;
    const timer = setInterval(
      () => setCooldown((left) => Math.max(0, left - 1)),
      1000,
    );
    return () => clearInterval(timer);
  }, [cooldown]);

  async function resend() {
    setFailure(null);
    try {
      await request(kind, email);
      setResent(true);
      setCooldown(assumedCooldownSeconds);
    } catch (error) {
      if (error instanceof ApiError && error.code === "RESEND_COOLING_DOWN") {
        // The server's number replaces the local estimate: this countdown is
        // the one that holds.
        setCooldown(
          error.retryAfterSeconds > 0
            ? error.retryAfterSeconds
            : assumedCooldownSeconds,
        );
        return;
      }
      setFailure(
        error instanceof ApiError
          ? error.message
          : "That did not send. Try again.",
      );
    }
  }

  return (
    <div>
      <p className="text-sm text-fg-2">
        We have sent a single-use link to the address below.
      </p>

      <dl className="mt-5 rounded-md border border-border bg-surface p-4 text-sm">
        <div className="flex justify-between gap-4">
          <dt className="text-fg-2">Sent to</dt>
          <dd className="font-medium">
            {email ? maskEmail(email) : "the address you gave"}
          </dd>
        </div>
        <div className="mt-2 flex justify-between gap-4">
          <dt className="text-fg-2">Subject</dt>
          <dd>{described[kind].subject}</dd>
        </div>
        <div className="mt-2 flex justify-between gap-4">
          <dt className="text-fg-2">Valid for</dt>
          <dd>{described[kind].validity}</dd>
        </div>
      </dl>

      <p className="mt-2 text-2xs text-fg-2">
        Wrong address? <Link href={changeAddressHref}>Start again</Link>.
      </p>

      {resent ? (
        <Banner tone="success">
          A new email is on its way. Only the newest one works.
        </Banner>
      ) : null}
      {failure ? <Banner tone="danger">{failure}</Banner> : null}

      <div className="mt-6">
        <p className="text-sm text-fg-2">
          Nothing yet? You can send it again once the cooldown finishes. We
          limit resends to stop mailbox flooding.
        </p>
        <div className="mt-3">
          <Button
            type="button"
            variant="secondary"
            disabled={cooldown > 0 || !email}
            onClick={() => void resend()}
          >
            {cooldown > 0 ? `Resend in ${cooldown}s` : "Resend email"}
          </Button>
        </div>
        {cooldown > 0 ? (
          // Politely, once a second would be a metronome in a screen reader.
          <p aria-live="polite" className="mt-2 text-2xs text-fg-2">
            A new email can be requested in {cooldown} seconds.
          </p>
        ) : null}
      </div>

      {kind === "otp" ? (
        <p className="mt-6 text-sm">
          Got the code? <Link href="/otp">Enter it here</Link>.
        </p>
      ) : null}
    </div>
  );
}
