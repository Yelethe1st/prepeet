"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { Button } from "@/shared/components";
import type { Speaker } from "@/lib/rtc/speakers";

import { emptyCaptions, foldCaptions, type CaptionState } from "./captions";
import {
  fetchLiveContext,
  replayEvents,
  type InterviewSession,
  type LiveContext,
} from "./api";

/**
 * The live interview surface: RTC-06, ported from the prototype's
 * candidate-session-live.html on top of RTC-01's shell and RTC-03's
 * recovery.
 *
 * Every state is words first: who is speaking, what the microphone is
 * doing, how the connection stands and how much time has passed are all
 * text, with colour and motion as reinforcement only. Push-to-talk works
 * by pointer and by keyboard and announces its changes. And nothing on
 * this screen scores anything: no articulation number, no filler count,
 * no correction appears during an answer, which the help panel says in so
 * many words.
 *
 * Captions are the durable timeline read back through replay, never a
 * parallel channel that could disagree with the evidence; the elapsed
 * clock rides the same segments, so a refresh resumes the timer from the
 * record rather than from zero.
 */

/** What the surface needs from the room: the microphone, nothing more. */
export interface MicrophoneControl {
  setMicrophoneEnabled(enabled: boolean): Promise<unknown>;
}

const CAPTION_POLL_MS = 2500;

export function InterviewSurface({
  sessionId,
  session,
  mic,
  subscribeSpeakers,
  paused,
  onEndConfirmed,
}: {
  sessionId: string;
  session: InterviewSession;
  mic: MicrophoneControl;
  subscribeSpeakers: (onChange: (speaker: Speaker) => void) => () => void;
  /** True while recovery runs: the timer stops with the connection. */
  paused: boolean;
  onEndConfirmed: () => void;
}) {
  const isScreening = session.mode === "screening";
  const plannedMinutes = session.config.minutes;
  const plannedSeconds = plannedMinutes * 60;

  const [context, setContext] = useState<LiveContext | null>(null);
  const [speaker, setSpeaker] = useState<Speaker>(null);
  const [muted, setMuted] = useState(false);
  const [pttOn, setPttOn] = useState(false);
  const [talking, setTalking] = useState(false);
  const [captionsOn, setCaptionsOn] = useState(true);
  const [captions, setCaptions] = useState<CaptionState>(emptyCaptions);
  const [elapsed, setElapsed] = useState(0);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [endOpen, setEndOpen] = useState(false);
  const [announcement, setAnnouncement] = useState("");
  const warnedFive = useRef(false);

  const announce = useCallback((text: string) => {
    setAnnouncement(text);
  }, []);

  // The persona speaking and the role and shape the session was configured
  // from, resolved from the catalogue because the config records only ids.
  useEffect(() => {
    let cancelled = false;
    Promise.resolve(fetchLiveContext())
      .then((resolved) => {
        if (!cancelled && resolved) {
          setContext(resolved);
        }
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => subscribeSpeakers(setSpeaker), [subscribeSpeakers]);

  // The microphone transmits when the mode says so: open microphone unless
  // muted, push-to-talk only while held. The room is told on every change.
  const transmitting = pttOn ? talking : !muted;
  useEffect(() => {
    void mic.setMicrophoneEnabled(transmitting);
  }, [mic, transmitting]);

  // Captions: poll the timeline from the cursor. Every rule about what a
  // replayed event means lives in the pure fold.
  useEffect(() => {
    let cancelled = false;
    let cursor = emptyCaptions.cursor;
    const poll = async (): Promise<void> => {
      try {
        const events = await replayEvents(
          sessionId,
          cursor.epoch,
          cursor.sequence,
        );
        if (cancelled || events.length === 0) {
          return;
        }
        setCaptions((state) => {
          const next = foldCaptions(state, events);
          cursor = next.cursor;
          return next;
        });
      } catch {
        // An unreachable poll costs one cycle; the next one catches up.
      }
    };
    void poll();
    const timer = setInterval(() => void poll(), CAPTION_POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [sessionId]);

  // The clock: one honest second at a time while the interview runs.
  // Reconnecting does not count against the candidate, so the tick pauses
  // with the connection (SES-05's rule, rendered).
  useEffect(() => {
    if (paused) {
      return;
    }
    const timer = setInterval(() => setElapsed((now) => now + 1), 1000);
    return () => clearInterval(timer);
  }, [paused]);

  useEffect(() => {
    if (!warnedFive.current && elapsed >= plannedSeconds - 300) {
      warnedFive.current = true;
      announce(
        `Five minutes left of the planned interview length. The planned length is ${plannedMinutes} minutes.`,
      );
    }
  }, [elapsed, plannedSeconds, plannedMinutes, announce]);

  const persona = context?.personas.find(
    (candidate) => candidate.id === session.config.persona,
  );
  const role = context?.roles.find(
    (candidate) => candidate.id === session.config.role,
  );
  const shape = context?.shapes.find(
    (candidate) => candidate.id === session.config.shape,
  );
  const personaName = persona?.name ?? "Your interviewer";
  const initials = personaName.slice(0, 2).toUpperCase();

  const setMutedAnnounced = useCallback(
    (next: boolean) => {
      setMuted(next);
      announce(next ? "Microphone muted." : "Microphone unmuted.");
    },
    [announce],
  );

  const setPtt = useCallback(
    (next: boolean) => {
      setPttOn(next);
      setTalking(false);
      // Push-to-talk implies muted until held; open microphone starts live.
      setMuted(next);
      announce(
        next
          ? "Push-to-talk enabled. Hold the space bar, or press and hold the microphone button, to speak."
          : "Open microphone enabled. Your microphone stays on unless you mute it.",
      );
    },
    [announce],
  );

  const startTalking = useCallback(() => {
    setTalking((already) => {
      if (!already) {
        announce("Microphone open.");
      }
      return true;
    });
  }, [announce]);

  const stopTalking = useCallback(() => {
    setTalking((was) => {
      if (was) {
        announce("Microphone closed.");
      }
      return false;
    });
  }, [announce]);

  // The shortcuts the help panel lists: M, C, Space (held), Escape.
  useEffect(() => {
    const editable = (target: EventTarget | null): boolean =>
      target instanceof HTMLElement &&
      (target.tagName === "INPUT" ||
        target.tagName === "TEXTAREA" ||
        target.isContentEditable);

    const onKeyDown = (event: KeyboardEvent): void => {
      if (editable(event.target)) {
        return;
      }
      if (event.key === "Escape") {
        setEndOpen((open) => !open);
        return;
      }
      if (event.repeat) {
        return;
      }
      if (event.key === "m" || event.key === "M") {
        if (!pttOn) {
          setMutedAnnounced(!muted);
        }
        return;
      }
      if (event.key === "c" || event.key === "C") {
        setCaptionsOn((on) => !on);
        return;
      }
      if (event.key === " " && pttOn && !endOpen) {
        event.preventDefault();
        startTalking();
      }
    };
    const onKeyUp = (event: KeyboardEvent): void => {
      if (event.key === " " && pttOn) {
        event.preventDefault();
        stopTalking();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("keyup", onKeyUp);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("keyup", onKeyUp);
    };
  }, [pttOn, muted, endOpen, setMutedAnnounced, startTalking, stopTalking]);

  const latest = captions.lines.at(-1);
  const blockedWhileSpeaking = speaker === "user" && !transmitting;

  const speakerLine =
    speaker === "ai"
      ? `${personaName} is speaking`
      : speaker === "user"
        ? `${personaName} is listening`
        : `${personaName} is waiting`;
  const youLine =
    speaker === "user"
      ? transmitting
        ? "You are speaking"
        : "You are speaking, but not transmitting"
      : "You are not speaking";
  const waveNote =
    speaker === "ai"
      ? `Voice activity: ${personaName} is speaking.`
      : speaker === "user"
        ? "Voice activity: your microphone is picking you up."
        : "Voice activity: nobody is speaking.";

  return (
    <div className="mx-auto flex min-h-[70vh] w-full max-w-[860px] flex-col gap-6">
      {/* Announcements: one polite region, fed by every state change. */}
      <p
        aria-live="polite"
        role="status"
        data-testid="announcer"
        className="sr-only"
      >
        {announcement}
      </p>

      {/* ── Top strip: time and connection, as text. ── */}
      <div className="flex flex-wrap items-center gap-3 border-b border-border pb-3 text-sm">
        <span
          role="timer"
          aria-label="Elapsed interview time"
          className="font-mono"
        >
          {mmss(elapsed)}
          <span className="text-fg-3"> / {mmss(plannedSeconds)}</span>
        </span>
        <span role="status" className="flex items-center gap-2">
          <span
            aria-hidden="true"
            className={`inline-block size-2 rounded-full ${paused ? "bg-danger" : "bg-success"}`}
          />
          {paused ? "Reconnecting…" : "Connected"}
        </span>
        <span className="ml-auto">
          <HelpMenu isScreening={isScreening} />
        </span>
      </div>

      {/* ── Stage ── */}
      <div className="flex flex-col items-center gap-1 text-center">
        <p className="text-2xs font-bold tracking-[0.12em] text-primary uppercase">
          {isScreening
            ? "Screening interview"
            : "Practice interview · not shared with employers"}
        </p>
        <h2 className="text-xl font-semibold">
          {shape?.name ?? "Live interview"}
        </h2>
        <p className="text-sm text-fg-3">
          {role ? `${role.title} · ` : ""}
          {plannedMinutes} minutes planned
        </p>
      </div>

      <div className="flex flex-col items-center gap-2">
        <span
          aria-hidden="true"
          className="grid size-16 place-items-center rounded-full border border-border bg-surface-2 text-lg font-semibold"
        >
          {initials}
        </span>
        <p className="text-md font-semibold">{personaName}</p>
        {persona ? <p className="text-sm text-fg-3">{persona.style}</p> : null}
      </div>

      <div
        role="status"
        className="flex flex-wrap justify-center gap-2 text-sm"
      >
        <span className="rounded-full border border-border px-3 py-1">
          {speakerLine}
        </span>
        <span className="rounded-full border border-border px-3 py-1">
          {youLine}
        </span>
      </div>
      <p className="text-center text-sm text-fg-3">{waveNote}</p>

      {blockedWhileSpeaking ? (
        <div
          role="alert"
          className="flex flex-wrap items-center gap-3 rounded-md border border-warning-border bg-warning-soft px-4 py-3 text-sm text-warning-fg"
        >
          <span>
            <strong>Your microphone is not transmitting.</strong>{" "}
            {pttOn
              ? `Push-to-talk is on and you are not holding the talk button. ${personaName} cannot hear you: hold Space to speak.`
              : `${personaName} cannot hear you. Press M or the microphone button to unmute.`}
          </span>
          {!pttOn ? (
            <Button
              type="button"
              size="sm"
              variant="secondary"
              onClick={() => setMutedAnnounced(false)}
            >
              Unmute
            </Button>
          ) : null}
        </div>
      ) : null}

      {/* ── Captions ── */}
      {captionsOn ? (
        <div
          aria-live="polite"
          aria-atomic="true"
          className="min-h-16 rounded-md border border-border bg-surface-2 px-4 py-3 text-sm"
          data-testid="caption"
        >
          <span className="font-semibold">
            {latest ? (latest.who === "ai" ? personaName : "You") : "Prepeet"}
          </span>{" "}
          <span>
            {latest
              ? latest.text
              : "Captions are on. What each person says will appear here."}
          </span>
        </div>
      ) : null}
      <div className="flex items-center justify-between text-sm">
        <p className="text-fg-3">
          Captions are generated live and may lag the audio slightly.
        </p>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => setHistoryOpen(true)}
        >
          Caption history ({captions.lines.length})
        </Button>
      </div>

      {/* ── Controls ── */}
      <div className="flex flex-wrap items-end justify-center gap-6 border-t border-border pt-4">
        <Control label="Captions">
          <Button
            type="button"
            size="lg"
            variant="secondary"
            aria-pressed={captionsOn}
            onClick={() => setCaptionsOn((on) => !on)}
          >
            {captionsOn ? "Turn captions off" : "Turn captions on"}
          </Button>
        </Control>

        <Control
          label={
            pttOn
              ? talking
                ? "Talking…"
                : "Hold to talk"
              : muted
                ? "Muted"
                : "Mute"
          }
        >
          <Button
            type="button"
            size="lg"
            variant="secondary"
            aria-pressed={pttOn ? talking : muted}
            aria-label={
              pttOn
                ? talking
                  ? "Talking. Release to stop."
                  : "Press and hold to talk"
                : muted
                  ? "Unmute your microphone"
                  : "Mute your microphone"
            }
            onClick={() => {
              if (!pttOn) {
                setMutedAnnounced(!muted);
              }
            }}
            onPointerDown={() => {
              if (pttOn) {
                startTalking();
              }
            }}
            onPointerUp={() => {
              if (pttOn) {
                stopTalking();
              }
            }}
            onPointerLeave={() => {
              if (pttOn && talking) {
                stopTalking();
              }
            }}
          >
            Microphone
          </Button>
        </Control>

        <Control label="Push-to-talk">
          <Button
            type="button"
            size="lg"
            variant="secondary"
            aria-pressed={pttOn}
            aria-label={
              pttOn
                ? "Switch back to open microphone"
                : "Switch to push-to-talk"
            }
            onClick={() => setPtt(!pttOn)}
          >
            {pttOn ? "Open microphone" : "Push-to-talk"}
          </Button>
        </Control>

        <Control label="End">
          <Button
            type="button"
            size="lg"
            variant="secondary"
            onClick={() => setEndOpen(true)}
          >
            End interview
          </Button>
        </Control>
      </div>
      {pttOn ? (
        <p className="text-center text-sm text-fg-3">
          Push-to-talk is on. Hold Space, or press and hold the microphone
          button, to speak. Release to stop.
        </p>
      ) : null}

      {historyOpen ? (
        <CaptionHistory
          lines={captions.lines}
          personaName={personaName}
          onClose={() => setHistoryOpen(false)}
        />
      ) : null}

      {endOpen ? (
        <EndDialog
          isScreening={isScreening}
          onCancel={() => setEndOpen(false)}
          onConfirm={onEndConfirmed}
        />
      ) : null}
    </div>
  );
}

function Control({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col items-center gap-1">
      {children}
      <span className="text-xs text-fg-3">{label}</span>
    </div>
  );
}

function HelpMenu({ isScreening }: { isScreening: boolean }) {
  const [open, setOpen] = useState(false);
  return (
    <span className="relative">
      <Button
        type="button"
        size="sm"
        variant="ghost"
        aria-expanded={open}
        onClick={() => setOpen((was) => !was)}
      >
        Help and shortcuts
      </Button>
      {open ? (
        <div className="absolute right-0 z-10 mt-2 w-[min(92vw,300px)] rounded-md border border-border bg-surface p-4 text-left text-sm shadow-lg">
          <p className="text-2xs font-bold tracking-[0.1em] text-fg-3 uppercase">
            Keyboard shortcuts
          </p>
          <ul className="mt-2 space-y-2 text-fg-2">
            <li className="flex justify-between gap-4">
              <span>Mute / unmute</span>
              <kbd>M</kbd>
            </li>
            <li className="flex justify-between gap-4">
              <span>Captions on / off</span>
              <kbd>C</kbd>
            </li>
            <li className="flex justify-between gap-4">
              <span>Hold to talk (push-to-talk)</span>
              <kbd>Space</kbd>
            </li>
            <li className="flex justify-between gap-4">
              <span>Leave the interview</span>
              <kbd>Esc</kbd>
            </li>
          </ul>
          <p className="mt-3 border-t border-border pt-3 text-fg-3">
            {isScreening
              ? "This is a screening interview. Your answers go to the hiring team. Prepeet will not show you a score or feedback for it."
              : "This is practice. There is no score on this screen and nothing here is shared with an employer."}
          </p>
        </div>
      ) : null}
    </span>
  );
}

function CaptionHistory({
  lines,
  personaName,
  onClose,
}: {
  lines: CaptionState["lines"];
  personaName: string;
  onClose: () => void;
}) {
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Caption history"
      className="fixed inset-0 z-[110] flex justify-end bg-overlay"
    >
      <div className="flex h-full w-[min(92vw,420px)] flex-col gap-3 overflow-y-auto border-l border-border bg-surface p-5">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold">Caption history</h2>
            <p className="text-sm text-fg-3">
              {lines.length === 0
                ? "Everything said so far in this session."
                : `${lines.length} line${lines.length === 1 ? "" : "s"} so far.`}
            </p>
          </div>
          <Button type="button" size="sm" variant="ghost" onClick={onClose}>
            Close
          </Button>
        </div>
        {lines.length === 0 ? (
          <p className="text-sm text-fg-2">
            Once the interview starts, every line of the conversation appears
            here so you can look back without breaking your flow.
          </p>
        ) : (
          <ol className="space-y-3">
            {lines.map((line) => (
              <li key={line.key} className="text-sm">
                <span className="font-semibold">
                  {line.who === "ai" ? personaName : "You"}
                </span>{" "}
                <span className="text-fg-2">{line.text}</span>
              </li>
            ))}
          </ol>
        )}
        <Button type="button" variant="secondary" onClick={onClose}>
          Back to the interview
        </Button>
      </div>
    </div>
  );
}

function EndDialog({
  isScreening,
  onCancel,
  onConfirm,
}: {
  isScreening: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const points = isScreening
    ? [
        "The hiring team receives exactly what you have said so far. The questions you have not reached are recorded as not answered.",
        "You cannot restart or retake this screening interview.",
        "Your audio and transcript up to this point are kept and submitted in full.",
      ]
    : [
        "You get a partial evaluation, based only on the questions you have answered.",
        "You can retry this interview as many times as you like.",
        "Everything you have said so far is kept in your practice history.",
      ];
  return (
    <div
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="end-h"
      className="fixed inset-0 z-[120] grid place-items-center bg-overlay p-6"
    >
      <div className="w-[min(92vw,460px)] space-y-4 rounded-xl border border-border bg-surface p-6 shadow-lg">
        <h2 id="end-h" className="text-base font-semibold">
          End this interview early?
        </h2>
        <p className="text-sm text-fg-2">If you end now:</p>
        <ul className="list-disc space-y-2 pl-5 text-sm text-fg-2">
          {points.map((point) => (
            <li key={point}>{point}</li>
          ))}
        </ul>
        <p className="rounded-md border border-border bg-surface-2 px-3 py-2 text-sm text-fg-2">
          {isScreening
            ? "Ending early is final. If your connection is the problem, use Retry now instead: the interview waits for you."
            : "Practice sessions can be repeated as often as you like. Nothing from practice mode is ever shared with an employer."}
        </p>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onCancel}>
            Cancel, keep going
          </Button>
          <Button type="button" variant="danger" onClick={onConfirm}>
            End interview
          </Button>
        </div>
      </div>
    </div>
  );
}

function mmss(totalSeconds: number): string {
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}
