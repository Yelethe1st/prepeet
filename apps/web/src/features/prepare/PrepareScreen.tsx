"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useRef, useState } from "react";
import type { ReactNode } from "react";

import { ApiError } from "@/lib/api/client";
import { Button } from "@/shared/components";
import {
  DelayedState,
  ErrorState,
  LoadingSurface,
  SkeletonBlock,
  SkeletonText,
} from "@/shared/states";

import { stashGrant } from "@/lib/rtc/grant";

import {
  fetchCatalogue,
  getInterview,
  getProfile,
  startInterview,
} from "./api";
import { realRunners, type CheckRunners } from "./checks";
import { startBlocker, type CheckStatus, type Checks } from "./gate";

/**
 * The prepare screen - SES-03, from the prototype's
 * candidate-session-prepare screen.
 *
 * The brief, the interviewer, the device checks, the accessibility switches
 * and the consent that actually blocks start. Nothing is recorded here: the
 * microphone check opens the microphone, measures level and closes it, and
 * the page says so in words wherever it matters. Start stays disabled until
 * the microphone and browser checks pass and the required recording consent
 * is given; the blocked state names exactly one missing thing and "take me
 * to what is missing" moves focus to it. The optional model-improvement
 * consent is separate, off by default, and nothing about the required
 * consent touches it.
 *
 * The accessibility switches arrive pre-set from the profile - captions and
 * extended thinking time honoured by default, which is PRO-01's promise -
 * and pressing start is SES-02's: this screen readies everything and holds
 * the gate.
 */
export function PrepareScreen({
  sessionId,
  runners = realRunners,
}: {
  sessionId: string;
  runners?: CheckRunners;
}) {
  const session = useQuery({
    queryKey: ["interview", sessionId],
    queryFn: () => getInterview(sessionId),
    // While composing, poll: readiness arrives from the workflow, not from
    // anything the person does here.
    refetchInterval: (query) =>
      query.state.data?.state === "composing" ? 2500 : false,
  });
  const catalogue = useQuery({
    queryKey: ["catalogue"],
    queryFn: fetchCatalogue,
  });
  const profile = useQuery({ queryKey: ["profile"], queryFn: getProfile });

  if (session.isPending || catalogue.isPending || profile.isPending) {
    return (
      <LoadingSurface label="your session">
        <SkeletonText width="50" />
        <SkeletonBlock />
        <SkeletonBlock />
      </LoadingSurface>
    );
  }
  if (session.isError || catalogue.isError || profile.isError) {
    const failure = session.isError
      ? session.error
      : catalogue.isError
        ? catalogue.error
        : profile.error;
    return (
      <ErrorState
        what="Your session could not be loaded"
        safe="The session itself is unaffected; only this view failed."
        reference={
          failure instanceof ApiError && failure.requestId
            ? failure.requestId
            : "none"
        }
        action={
          <Button type="button" onClick={() => void session.refetch()}>
            Try again
          </Button>
        }
      />
    );
  }

  if (session.data.state === "composing") {
    return (
      <DelayedState what="Your interview is still being composed">
        <p>This page opens itself the moment it is ready.</p>
      </DelayedState>
    );
  }
  if (session.data.state === "composition_failed") {
    return (
      <ErrorState
        what="Your interview could not be composed"
        safe="Nothing you entered is lost, and nothing was recorded. Starting a fresh setup takes a minute."
        reference={session.data.failure_code ?? "none"}
        action={null}
      />
    );
  }

  return (
    <Prepared
      session={session.data}
      catalogue={catalogue.data}
      profile={profile.data}
      runners={runners}
    />
  );
}

function Prepared({
  session,
  catalogue,
  profile,
  runners,
}: {
  session: Awaited<ReturnType<typeof getInterview>>;
  catalogue: Awaited<ReturnType<typeof fetchCatalogue>>;
  profile: Awaited<ReturnType<typeof getProfile>>;
  runners: CheckRunners;
}) {
  const [checks, setChecks] = useState<Checks>(() => ({
    mic: "pending",
    speaker: "pending",
    net: "pending",
    // The browser check needs no interaction, so it answers immediately.
    browser: runners.browser(),
  }));
  const [requiredConsent, setRequiredConsent] = useState(false);
  const [improveConsent, setImproveConsent] = useState(false);

  // The accessibility promise: the switches arrive pre-set from the
  // profile. The person can still change them for this session.
  const [captions, setCaptions] = useState(profile.captions);
  const [pushToTalk, setPushToTalk] = useState(false);
  const [extraTime, setExtraTime] = useState(profile.extended_time);

  const micButton = useRef<HTMLButtonElement>(null);
  const consentBox = useRef<HTMLInputElement>(null);
  const announceRef = useRef<HTMLParagraphElement>(null);
  const router = useRouter();

  // The gate opening into the interview: start, stash the one-use grant for
  // the live route, go. Every refusal arrives with its own code and the
  // server's own words, which are written for the person.
  const start = useMutation({
    mutationFn: () => startInterview(session.id),
    onSuccess: (started) => {
      stashGrant({
        sessionId: session.id,
        url: started.realtime.url,
        room: started.realtime.room,
        token: started.realtime.token,
        expiresAt: started.realtime.expires_at,
      });
      router.push(`/session/${session.id}`);
    },
  });

  const setCheck = (check: keyof Checks, status: CheckStatus) =>
    setChecks((previous) => ({ ...previous, [check]: status }));

  const runMic = async () => {
    setCheck("mic", "testing");
    setCheck("mic", await runners.mic());
  };
  const runSpeaker = async () => {
    setCheck("speaker", "testing");
    await runners.speaker();
    setCheck("speaker", "confirm");
  };
  const runNet = async () => {
    setCheck("net", "testing");
    setCheck("net", await runners.net());
  };

  const blocked = startBlocker(checks, requiredConsent);

  const focusProblem = () => {
    if (!blocked) {
      return;
    }
    if (blocked.target === "mic") {
      micButton.current?.focus();
    }
    if (blocked.target === "consent") {
      consentBox.current?.focus();
    }
  };

  const role = catalogue.roles.find((each) => each.id === session.config.role);
  const shape = catalogue.shapes.find(
    (each) => each.id === session.config.shape,
  );
  const persona = catalogue.personas.find(
    (each) => each.id === session.config.persona,
  );
  const passed = Object.values(checks).filter(
    (status) => status === "pass",
  ).length;

  return (
    <div className="max-w-[720px] space-y-6">
      <Card heading="Your brief">
        <p className="text-sm">
          <strong>{role?.title}</strong> · {shape?.name} ·{" "}
          {session.config.minutes} minutes
        </p>
        <p className="mt-2 text-sm text-fg-2">{shape?.description}</p>
        <h3 className="mt-4 text-sm font-semibold">What will be assessed</h3>
        <ul className="mt-1 list-disc pl-5 text-sm text-fg-2">
          {role?.competencies.map((competency) => (
            <li key={competency}>{competency}</li>
          ))}
        </ul>
        <p className="mt-3 text-sm text-fg-2">
          Nothing is recorded until you press start, and a pause to think costs
          you nothing - the interviewer waits through silence.
        </p>
      </Card>

      <Card heading="Who is interviewing you">
        <p className="text-sm font-semibold">
          {persona?.name} · {persona?.style}
        </p>
        <p className="mt-1 text-sm text-fg-2">{persona?.description}</p>
        <p className="mt-1 text-sm text-fg-2">
          A persona is an interview style, never a judgement about you.
        </p>
      </Card>

      <Card heading="Device check">
        <p className="text-sm text-fg-2">
          The microphone check must pass before you can start. The rest are
          strongly recommended.
        </p>
        <ul className="mt-3 space-y-2">
          <CheckRow
            label="Microphone"
            requirement="Required"
            status={checks.mic}
            statusWords={{
              pending: "Not tested yet",
              testing: "Listening. Say a sentence out loud.",
              pass: "We heard you clearly.",
              fail: "No audio arrived. Check the browser's microphone permission, then re-test.",
            }}
            action={
              <Button
                ref={micButton}
                type="button"
                size="sm"
                variant="secondary"
                onClick={() => void runMic()}
                busy={checks.mic === "testing"}
              >
                Test microphone
              </Button>
            }
          />
          <CheckRow
            label="Speaker"
            requirement="Recommended"
            status={checks.speaker}
            statusWords={{
              pending: "Not tested yet",
              testing: "Playing a tone.",
              confirm: "Did you hear the tone?",
              pass: "You heard the tone.",
              fail: "Check your output device, then re-test.",
            }}
            action={
              checks.speaker === "confirm" ? (
                <span className="flex gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="secondary"
                    onClick={() => setCheck("speaker", "pass")}
                  >
                    I heard it
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="secondary"
                    onClick={() => setCheck("speaker", "fail")}
                  >
                    I heard nothing
                  </Button>
                </span>
              ) : (
                <Button
                  type="button"
                  size="sm"
                  variant="secondary"
                  onClick={() => void runSpeaker()}
                  busy={checks.speaker === "testing"}
                >
                  Test speaker
                </Button>
              )
            }
          />
          <CheckRow
            label="Connection"
            requirement="Recommended"
            status={checks.net}
            statusWords={{
              pending: "Not tested yet",
              testing: "Measuring.",
              pass: "Fast enough for live audio.",
              fail: "Slow right now. The interview still works; expect delays.",
            }}
            action={
              <Button
                type="button"
                size="sm"
                variant="secondary"
                onClick={() => void runNet()}
                busy={checks.net === "testing"}
              >
                Test connection
              </Button>
            }
          />
          <CheckRow
            label="Browser"
            requirement="Required"
            status={checks.browser}
            statusWords={{
              pending: "Checking.",
              pass: "This browser can run the interview.",
              fail: "This browser cannot capture audio. Open this page in a current Chrome, Edge, Firefox or Safari.",
            }}
            action={
              <Button
                type="button"
                size="sm"
                variant="secondary"
                onClick={() => setCheck("browser", runners.browser())}
              >
                Re-check
              </Button>
            }
          />
        </ul>
        <p role="status" aria-live="polite" className="mt-3 text-sm text-fg-2">
          {passed} of 4 checks passed.
        </p>
      </Card>

      <Card heading="Quiet space & accessibility">
        <p className="text-sm text-fg-2">
          Find somewhere you can talk out loud without being interrupted. These
          arrive pre-set from your profile and apply to this session.
        </p>
        <div className="mt-3 space-y-2">
          <Toggle
            label="Live captions"
            hint="Every question appears as text while it is spoken."
            checked={captions}
            onChange={setCaptions}
          />
          <Toggle
            label="Push to talk"
            hint="Hold the space bar while answering instead of leaving the microphone open."
            checked={pushToTalk}
            onChange={setPushToTalk}
          />
          <Toggle
            label="Extra thinking time"
            hint="Doubles the silence the interviewer waits through. It does not affect your evaluation and is never reported to anyone."
            checked={extraTime}
            onChange={setExtraTime}
          />
        </div>
      </Card>

      <Card heading="Consent">
        <p className="text-sm text-fg-2">
          Nothing has been recorded yet. Nothing will be until you agree and
          press start.
          <span className="ml-2 rounded-full border border-border px-2 py-0.5 font-mono text-2xs text-fg-3">
            consent v{session.consent_version}
          </span>
        </p>
        <div className="mt-3 space-y-3">
          <label className="flex cursor-pointer items-start gap-3">
            <input
              ref={consentBox}
              type="checkbox"
              checked={requiredConsent}
              onChange={(event) => setRequiredConsent(event.target.checked)}
              className="mt-1"
              aria-describedby="consent-required-hint"
            />
            <span>
              <span className="block text-sm font-semibold">
                Record and transcribe this interview so it can be evaluated.{" "}
                <span className="text-danger">Required</span>
              </span>
              <span
                id="consent-required-hint"
                className="block text-sm text-fg-2"
              >
                Your voice is recorded, turned into a transcript, and evaluated
                against the rubric for this session. Both are stored in your own
                account; no employer can see a practice session.
              </span>
            </span>
          </label>
          <label className="flex cursor-pointer items-start gap-3">
            <input
              type="checkbox"
              checked={improveConsent}
              onChange={(event) => setImproveConsent(event.target.checked)}
              className="mt-1"
              aria-describedby="consent-improve-hint"
            />
            <span>
              <span className="block text-sm font-semibold">
                Let Prepeet use this session to improve its interviewing.{" "}
                <span className="font-normal text-fg-3">Optional</span>
              </span>
              <span
                id="consent-improve-hint"
                className="block text-sm text-fg-2"
              >
                Off unless you switch it on, and never bundled with the consent
                above. You can change your mind for past sessions at any time in
                privacy settings.
              </span>
            </span>
          </label>
        </div>
      </Card>

      <Card heading="Ready?">
        <p className="text-sm text-fg-2">
          Recording begins the moment you press start. The first question comes
          about five seconds later.
        </p>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Button
            type="button"
            disabled={blocked !== null}
            aria-describedby="start-blocked"
            busy={start.isPending}
            onClick={() => start.mutate()}
          >
            Start interview
          </Button>
        </div>
        {start.isError ? (
          <p role="alert" className="mt-2 text-sm font-semibold text-danger">
            {start.error instanceof ApiError
              ? start.error.message
              : "Starting did not work. Nothing was recorded; try again."}
          </p>
        ) : null}
        <p
          id="start-blocked"
          ref={announceRef}
          tabIndex={-1}
          className="mt-2 text-sm text-fg-2"
        >
          {blocked
            ? blocked.message
            : `All required checks passed. ${passed} of 4 checks complete.`}
        </p>
        {blocked && blocked.target !== "browser" ? (
          <Button
            type="button"
            size="sm"
            variant="secondary"
            onClick={focusProblem}
          >
            Take me to what is missing
          </Button>
        ) : null}
      </Card>
    </div>
  );
}

function Card({ heading, children }: { heading: string; children: ReactNode }) {
  return (
    <section
      aria-label={heading}
      className="rounded-md border border-border bg-surface px-4 py-4"
    >
      <h2 className="text-base font-semibold">{heading}</h2>
      <div className="mt-2">{children}</div>
    </section>
  );
}

function CheckRow({
  label,
  requirement,
  status,
  statusWords,
  action,
}: {
  label: string;
  requirement: string;
  status: CheckStatus;
  statusWords: Partial<Record<CheckStatus, string>>;
  action: ReactNode;
}) {
  return (
    <li
      className={`rounded-md border px-3 py-2 ${
        status === "pass"
          ? "border-success-border"
          : status === "fail"
            ? "border-danger-border"
            : "border-border"
      }`}
    >
      <div className="flex flex-wrap items-center gap-3">
        <span className="flex-1">
          <span className="block text-sm font-semibold">
            {label}{" "}
            <span className="font-normal text-fg-3">· {requirement}</span>
          </span>
          <span className="block text-sm text-fg-2">{statusWords[status]}</span>
        </span>
        {action}
      </div>
    </li>
  );
}

function Toggle({
  label,
  hint,
  checked,
  onChange,
}: {
  label: string;
  hint: string;
  checked: boolean;
  onChange: (value: boolean) => void;
}) {
  return (
    <label className="flex cursor-pointer items-start gap-3">
      <input
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        className="mt-1"
      />
      <span>
        <span className="block text-sm font-semibold">{label}</span>
        <span className="block text-sm text-fg-2">{hint}</span>
      </span>
    </label>
  );
}
