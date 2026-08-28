import {
  Accessibility,
  Ban,
  Captions,
  ClipboardCheck,
  GitCompare,
  Globe2,
  GraduationCap,
  Hand,
  Layers,
  Link as LinkGlyph,
  Mic,
  Quote,
  RotateCcw,
  Ruler,
  Scale,
  ShieldCheck,
  Split,
  type LucideIcon,
} from "lucide-react";

/**
 * The marketing page's copy, ported from screens/index.html.
 *
 * It is data rather than markup because the page is nine lists and a table.
 * Written into the components it would be nine components nobody can read, and
 * a copy change would mean editing JSX in nine places to find out whether the
 * words still fit.
 *
 * Three deviations from the prototype are recorded here, because this is where
 * they were made and a reviewer looking for them should find them together.
 *
 * 1. No em dashes. The product's copy rule forbids them, and the prototype's
 *    copy uses them heavily. Every one is a colon, a comma or a full stop here.
 *    The words are otherwise the prototype's, including its British spelling
 *    and its typographic quotes.
 *
 * 2. Links point at routes that exist. The prototype is 56 HTML files and links
 *    freely between them; most of those screens are not ported yet. A link to a
 *    screen that does not exist is a 404 from the front page, so where the
 *    destination is not built the link becomes an anchor to the section of this
 *    page that answers the same question, and where there is no such section it
 *    is dropped. Nothing here links to a route that is not in the application.
 *    Each one is marked below so the mapping can be undone as screens land.
 *
 * 3. The prototype's mobile menu and footer link to its own prototype index and
 *    design-system page. Those are development artefacts and are not shipped.
 */

/** A navigation destination. In-page anchors and real routes, nothing else. */
export interface NavLink {
  label: string;
  href: string;
}

export const primaryNav: NavLink[] = [
  { label: "Product", href: "#product" },
  { label: "Use cases", href: "#use-cases" },
  { label: "How it works", href: "#how" },
  { label: "Evidence", href: "#evidence" },
  { label: "FAQ", href: "#faq" },
];

/* ------------------------------------------------------------------ hero -- */

export const hero = {
  eyebrow: "Realtime voice interviews · Practice & screening",
  headingBefore: "Interviews that leave ",
  headingEmphasis: "evidence",
  headingAfter: ", not impressions.",
  leadBefore:
    "Prepeet holds a real spoken conversation: it listens, follows up, and pushes back when an answer is thin. Candidates practise with honest coaching. Recruiters get an evaluation where every competency score points back to a sentence the candidate actually said, or says ",
  leadEmphasis: "insufficient evidence",
  leadAfter: " and stops.",
  /** The prototype's second call to action went to the recruiter dashboard, which is not ported. */
  assurances: [
    { glyph: Quote, text: "No score without a linked quote" },
    { glyph: Globe2, text: "EU and US data residency" },
    { glyph: Accessibility, text: "WCAG 2.2 AA, captions and push-to-talk" },
  ] as { glyph: LucideIcon; text: string }[],
  frameDescription:
    "Preview of a live Prepeet interview: practice mode, question 4 of 6, 18 minutes 42 seconds elapsed, interviewer persona Ravi speaking, with a live caption of the question being asked.",
  framePath: "/candidate/session/ses_7Kq2XA/live",
  framePhase: "Phase 2 · Technical depth",
  frameTimer: "18:42",
  frameSpeaker: "Ravi is speaking",
  frameCaptionWho: "Ravi · question 4 of 6",
  frameCaption:
    "Say Monday at 08:00 you get thirty thousand booking attempts a minute against overlapping slots. How do you stop double booking?",
  frameHint: "Captions on · Push-to-talk available",
};

/* ----------------------------------------------------------------- proof -- */

export const proof = {
  heading: "Organisations using Prepeet",
  lead: "Interview teams in health systems, software companies, school trusts, finance and commercial sales run Prepeet alongside, never instead of, their own interviewers.",
  logos: [
    { initials: "NH", name: "Northwind Health System" },
    { initials: "OL", name: "Orbital Labs" },
    { initials: "MS", name: "Meridian Schools Trust" },
    { initials: "CC", name: "Caldera Capital" },
    { initials: "BC", name: "Brightpath Commercial" },
    { initials: "SM", name: "Saltmarsh Care Trust" },
  ],
  stats: [
    { value: "41,200", label: "Interviews completed in the last 12 months" },
    {
      value: "100%",
      label: "Screen evaluations reviewed by a person before any decision",
    },
    { value: "0", label: "Scores published without a linked transcript quote" },
    {
      value: "6",
      label: "Domains with maintained rubrics and calibration sets",
    },
  ],
};

/* ------------------------------------------------------------- use cases -- */

export interface UseCase {
  domain: string;
  role: string;
  competency: string;
  probe: string;
  question: string;
}

export const useCases = {
  eyebrow: "Use cases",
  heading: "Six domains, six rubrics, six very different conversations",
  lead: "A nursing escalation question and a systems design question are not the same interview. Each domain has its own competency set, its own follow-up logic and its own anchored rubric, maintained with subject-matter reviewers.",
  /** The prototype linked "rubric management" here. That screen is not ported, so it is a statement rather than a link. */
  footnote:
    "Rubrics are versioned per tenant and editable by the hiring team, with every level anchored to written criteria and worked examples.",
  items: [
    {
      domain: "Software engineering",
      role: "Senior Backend Engineer",
      competency: "debugging & incident response",
      probe: "Does the candidate describe the diagnosis, or only the fix?",
      question:
        "“Your booking service is healthy but writes to the patient record are timing out. Walk me through your first ten minutes.”",
    },
    {
      domain: "Product management",
      role: "Senior Product Manager",
      competency: "prioritisation",
      probe:
        "Is there a stated trade-off, or just a list of everything that matters?",
      question:
        "“You have one engineer for six weeks and three customers each threatening to leave over a different gap. How do you choose, and what do you tell the other two?”",
    },
    {
      domain: "Nursing",
      role: "Registered Nurse, Intensive Care",
      competency: "escalation & handover",
      probe: "Who gets called, when, and what is said in the first sentence.",
      question:
        "“Your ventilated patient’s mean arterial pressure drops to 52 and the registrar is in theatre. Talk me through what you do and who you escalate to.”",
    },
    {
      domain: "Teaching",
      role: "Secondary Mathematics Teacher",
      competency: "assessment & feedback",
      probe: "Does the plan change because of the data, or in spite of it?",
      question:
        "“Two thirds of your Year 9 class got the same simultaneous equations question wrong. What does tomorrow’s lesson look like, and how do you know it worked?”",
    },
    {
      domain: "Sales",
      role: "Enterprise Account Executive",
      competency: "commercial qualification",
      probe: "Is the deal qualified on paper, or on hope?",
      question:
        "“Your champion loves the product, but procurement says the security review adds nine weeks. What do you do this week, and what do you change in the forecast?”",
    },
    {
      domain: "Finance",
      role: "Financial Analyst, FP&A",
      competency: "variance analysis",
      probe:
        "From the number, to the cause, to a sentence an operator can act on.",
      question:
        "“Theatre staffing has come in 11% over budget for the third month running. How do you find out why, and what is the one line you put in front of the COO?”",
    },
  ] as UseCase[],
};

/* -------------------------------------------------------------- features -- */

export const features = {
  eyebrow: "Product",
  heading: "Built for the part everyone skips: the write-up",
  lead: "Running the interview is the easy half. The hard half is producing something a hiring panel can argue with, and a candidate can appeal.",
  items: [
    {
      glyph: Mic,
      title: "Realtime voice, not a form with a microphone",
      body: "Sub-second turn-taking over WebRTC. The interviewer hears you pause, waits, and asks the follow-up your answer actually invited. You can interrupt it; it will let you.",
    },
    {
      glyph: Quote,
      title: "Evidence-linked evaluation",
      body: "Every competency score is assembled from spans of the transcript, each with a timestamp and a playable audio range. Click a score, land on the sentence. Nothing is scored from tone, pace or hesitation.",
    },
    {
      glyph: Ruler,
      title: "Calibration and rubric anchors",
      body: "Each rubric level carries a written anchor and two real example answers. Tenants run a calibration set before go-live and re-run it whenever a rubric or model version changes; drift is reported, not hidden.",
    },
    {
      glyph: GraduationCap,
      title: "Practice coaching that is specific",
      body: "Practice mode returns per-answer feedback, a rewritten version of your own answer, and the one question most likely to expose the same gap next time. Retry any question as many times as you like.",
    },
    {
      glyph: ClipboardCheck,
      title: "Recruiter review tools",
      body: "Side-by-side candidate comparison on the same rubric, coverage and confidence flags, structured reviewer notes, and a decision that is recorded as a person’s decision, with their name on it.",
    },
    {
      glyph: ShieldCheck,
      title: "Multi-tenant governance and audit",
      body: "Per-tenant data residency, retention windows, SSO and SCIM, role-scoped access down to a single requisition, and an append-only audit log covering every view, export and override.",
    },
  ] as { glyph: LucideIcon; title: string; body: string }[],
};

/* ---------------------------------------------------------- how it works -- */

/**
 * What a cell in the visibility table says, and how it reads at a glance.
 *
 * Never colour alone. Each mark draws a different glyph as well as a different
 * colour, and every cell carries the sentence it means, so the table is
 * readable with no colour perception at all and correct when read aloud.
 */
export type Mark = "yes" | "no" | "na" | "limited";

export interface VisibilityRow {
  what: string;
  practice: { mark: Mark; text: string };
  screen: { mark: Mark; text: string };
  recruiter: { mark: Mark; text: string };
}

export const howItWorks = {
  eyebrow: "How it works",
  heading: "Four steps, then the part that matters: who sees what",
  lead: "Same interview engine, same rubric, two very different products on either side of it.",
  steps: [
    {
      title: "Set the interview up",
      body: "A recruiter picks a role, a rubric version, a shape (behavioural, technical, case, panel) and a time box, then sends an invitation. A practice candidate picks their target role and an interviewer persona instead.",
    },
    {
      title: "Have the conversation",
      body: "15 to 60 minutes of live voice. Microphone check first, captions throughout, push-to-talk if the room is noisy, and a hard time box the candidate can see counting down.",
    },
    {
      title: "Extract the evidence",
      body: "The transcript is segmented into answers, and each answer is matched to rubric anchors. Spans are tagged as supporting, contradicting, unverified claim, or gap. Thin competencies are marked, not guessed.",
    },
    {
      title: "Review and decide",
      body: "Practice candidates get coaching and a progression view. Recruiters get an evidence pack and make the decision themselves. Prepeet never advances, rejects or ranks anyone on its own.",
    },
  ],
  tableHeading: "Practice mode vs screen mode: exactly who sees what",
  tableLead:
    "This boundary is enforced in the product, not by policy. In screen mode, a candidate is never shown a score, a band, a competency chart, coaching, a recommendation or a reviewer note: only confirmation that their interview was submitted.",
  tableCaption:
    "What is visible to a practice candidate, a screen candidate and a recruiter reviewing a screen interview.",
  columns: [
    "What",
    "Practice candidate",
    "Screen candidate",
    "Recruiter reviewing a screen",
  ],
  rows: [
    {
      what: "The live conversation",
      practice: { mark: "yes", text: "Yes: identical engine and personas" },
      screen: { mark: "yes", text: "Yes: identical engine and personas" },
      recruiter: { mark: "yes", text: "Audio and full transcript, afterwards" },
    },
    {
      what: "Overall score and band",
      practice: { mark: "yes", text: "Shown immediately after processing" },
      screen: { mark: "no", text: "Never shown" },
      recruiter: { mark: "yes", text: "Shown with confidence and coverage" },
    },
    {
      what: "Per-competency chart and confidence range",
      practice: { mark: "yes", text: "Shown, tracked over time" },
      screen: { mark: "no", text: "Never shown" },
      recruiter: {
        mark: "yes",
        text: "Shown, including “insufficient evidence” rows",
      },
    },
    {
      what: "Per-answer coaching and model rewrites",
      practice: { mark: "yes", text: "Shown for every answer" },
      screen: { mark: "no", text: "Never generated at all" },
      recruiter: { mark: "no", text: "Not shown: coaching is practice-only" },
    },
    {
      what: "Retry a question",
      practice: { mark: "yes", text: "Unlimited retries" },
      screen: { mark: "no", text: "No: one submission per invitation" },
      recruiter: {
        mark: "na",
        text: "Can re-issue the invitation if something went wrong",
      },
    },
    {
      what: "Evidence quotes linked to the transcript",
      practice: { mark: "yes", text: "Shown against their own answers" },
      screen: { mark: "no", text: "Never shown" },
      recruiter: {
        mark: "yes",
        text: "Every score traceable to a timestamped span",
      },
    },
    {
      what: "Reviewer notes and decision",
      practice: { mark: "na", text: "Not applicable: no reviewer involved" },
      screen: { mark: "no", text: "Never shown" },
      recruiter: {
        mark: "yes",
        text: "Written by them, attributed and audit-logged",
      },
    },
    {
      what: "What the end of the session looks like",
      practice: {
        mark: "yes",
        text: "Full report, plus a suggested next session",
      },
      screen: {
        mark: "limited",
        text: "“Your interview was submitted”, and nothing more",
      },
      recruiter: {
        mark: "yes",
        text: "Evidence pack lands in the review queue",
      },
    },
    {
      what: "Appeal or human re-review",
      practice: { mark: "na", text: "Not applicable" },
      screen: {
        mark: "yes",
        text: "Can request a human review of the interview",
      },
      recruiter: {
        mark: "yes",
        text: "Appeals arrive in a tracked queue with a deadline",
      },
    },
  ] as VisibilityRow[],
};

/* -------------------------------------------------------------- realtime -- */

export interface TranscriptTurn {
  speaker: string;
  initials: string;
  interviewer: boolean;
  at: string;
  seconds: string;
  note?: string;
  noteTone?: "outline" | "warning";
  current?: boolean;
  text: string;
}

export const realtime = {
  eyebrow: "The live conversation",
  heading: "It behaves like an interviewer, including when you cut it off",
  lead: "Most “AI interviews” are a queue of recorded questions. Prepeet holds one continuous conversation: it tracks what you have already covered, notices when two answers disagree, and asks about it in the session, while you can still respond.",
  points: [
    {
      glyph: Split,
      lead: "Interruption handling.",
      body: "Start talking and the interviewer stops mid-sentence, keeps your first words, and resumes from where you took over. Barge-in is on by default and can be disabled per tenant.",
    },
    {
      glyph: Layers,
      lead: "Phase progression.",
      body: "Warm-up, core competencies, depth probes, candidate questions, close. Each phase has its own time budget, visible to the candidate the whole way through.",
    },
    {
      glyph: Captions,
      lead: "Live captions.",
      body: "Every word the interviewer says is captioned in real time, with an adjustable text size and a full scrollback.",
    },
    {
      glyph: Hand,
      lead: "Push-to-talk fallback.",
      body: "On a noisy line or a shared room, hold the space bar (or the on-screen button) to talk. Barge-in turns off automatically in this mode.",
    },
    {
      glyph: RotateCcw,
      lead: "Reconnection.",
      body: "If the network drops, the session pauses, the clock stops, and the candidate has ten minutes to rejoin on the same link. Nothing already said is lost.",
    },
  ] as { glyph: LucideIcon; lead: string; body: string }[],
  transcriptTitle: "Transcript excerpt",
  transcriptMeta: "Systems design deep-dive · persona Ravi · 21 Aug 2026",
  turns: [
    {
      speaker: "Ravi",
      initials: "RA",
      interviewer: true,
      at: "02:05",
      seconds: "PT2M5S",
      text: "What happens when the database that holds that constraint is the thing under pressure?",
    },
    {
      speaker: "Daniel",
      initials: "DO",
      interviewer: false,
      at: "02:14",
      seconds: "PT2M14S",
      text: "Then I’d shard by clinic, because bookings never cross clinics. Each shard owns its own slots, so the uniqueness constraint stays local.",
    },
    {
      speaker: "Ravi",
      initials: "RA",
      interviewer: true,
      at: "02:41",
      seconds: "PT2M41S",
      note: "Interrupted the answer",
      noteTone: "outline",
      text: "And if a clinic is itself the hotspot?",
    },
    {
      speaker: "Daniel",
      initials: "DO",
      interviewer: false,
      at: "02:47",
      seconds: "PT2M47S",
      text: "Probably a queue in front of it. I’d let requests line up and process them in order.",
    },
    {
      speaker: "Ravi",
      initials: "RA",
      interviewer: true,
      at: "05:16",
      seconds: "PT5M16S",
      note: "Consistency follow-up",
      noteTone: "warning",
      current: true,
      text: "Earlier you said you shard by clinic. A moment ago you described a global queue. Which is it?",
    },
    {
      speaker: "Daniel",
      initials: "DO",
      interviewer: false,
      at: "05:24",
      seconds: "PT5M24S",
      text: "Per-shard queue, not global. I said it loosely the first time.",
    },
  ] as TranscriptTurn[],
  transcriptFootnote:
    "The follow-up at 05:16 was generated because two answers conflicted, not because of how the candidate sounded. Prepeet does not score tone, accent, pace or hesitation.",
};

/* -------------------------------------------------------------- evidence -- */

export const evidence = {
  eyebrow: "Evidence and feedback",
  heading: "A score you can’t trace back to a sentence isn’t a score",
  leadBefore:
    "Prepeet builds each competency from tagged spans of the candidate’s own words. Open any score and you get the quote, the timestamp and the audio. Where the conversation never produced enough material, the competency reads ",
  leadEmphasis: "insufficient evidence",
  leadAfter: ": a deliberate blank, not a low number.",
  points: [
    {
      glyph: LinkGlyph,
      lead: "Traceable by construction.",
      body: "A competency cannot be published without at least three independent supporting spans; below that it is withheld.",
    },
    {
      glyph: GitCompare,
      lead: "Contradictions are surfaced, not averaged.",
      body: "Conflicting answers appear as their own evidence type so a reviewer can judge them.",
    },
    {
      glyph: Ban,
      lead: "Nothing inferred about the person.",
      body: "No personality inference, no emotion or honesty detection, no prediction of how someone will perform in the job.",
    },
    {
      glyph: Scale,
      lead: "Confidence ranges, always visible.",
      body: "Every score carries the interval it was derived from, so a 66 with a wide range never reads like a 66 with a narrow one.",
    },
  ] as { glyph: LucideIcon; lead: string; body: string }[],
  /**
   * The worked example: one competency with enough evidence, one without.
   *
   * The quote is split into parts so a tagged span can be marked in place. Each
   * tagged span carries what its tag means, which the prototype puts in a
   * tooltip; a tooltip is not readable by touch or by keyboard alone, so the
   * meaning is text here.
   */
  supportingCard: {
    title: "Patient-safety reasoning",
    band: "Supporting evidence",
    parts: [
      { text: "“I would " },
      {
        text: "fail closed: reject the booking rather than confirm something the clinical record does not know about",
        kind: "supporting" as const,
        meaning:
          "Supporting evidence: chooses the safe failure direction and says why.",
      },
      { text: ". Then queue a repair job and alert. " },
      {
        text: "Confirming a booking the record never received is the worse outcome for a patient.",
        kind: "claim" as const,
        meaning:
          "Unverified claim: a stated judgement, not yet backed by a concrete example.",
      },
      { text: "”" },
    ],
    meta: ["ses_7Kq2XA", "04:22 to 04:41", "turn 12", "audio available"],
  },
  gapCard: {
    title: "Prioritisation under pressure",
    band: "Gap: insufficient evidence",
    quote:
      "“Probably a queue in front of it. I’d let requests line up and process them in order.”",
    note: "One span, no trade-off stated and no constraint named. Prepeet records the gap and declines to score the competency rather than inferring a level from a single thin answer.",
    meta: ["ses_7Kq2XA", "02:47 to 02:56", "turn 7", "1 of 3 spans required"],
  },
  legendHeading: "Evidence types",
  legend: [
    { kind: "supporting", text: "Supporting: backs the rubric anchor" },
    {
      kind: "contradiction",
      text: "Contradiction: conflicts with an earlier answer",
    },
    { kind: "claim", text: "Claim: asserted but unevidenced" },
    { kind: "gap", text: "Gap: the rubric point was never reached" },
  ] as {
    kind: "supporting" | "contradiction" | "claim" | "gap";
    text: string;
  }[],
  signalHeading: "Competency signal · confidence range shown",
  competencies: [
    {
      name: "Systems design",
      detail: "6 evidence spans · Solid",
      score: "78",
      fill: 78,
      intervalStart: 70,
      intervalWidth: 14,
    },
    {
      name: "Debugging & incident response",
      detail: "4 evidence spans · Solid",
      score: "66",
      fill: 66,
      intervalStart: 56,
      intervalWidth: 18,
    },
    {
      name: "Prioritisation under pressure",
      detail: "1 evidence span · Insufficient evidence",
      score: null,
      fill: 100,
      intervalStart: 0,
      intervalWidth: 0,
    },
  ] as {
    name: string;
    detail: string;
    score: string | null;
    fill: number;
    intervalStart: number;
    intervalWidth: number;
  }[],
  chartSummary:
    "Text summary: systems design scores 78 out of 100 (Solid) with a confidence range of 70 to 84, from 6 evidence spans. Debugging and incident response scores 66 (Solid) with a wider range of 56 to 74, from 4 spans. Prioritisation under pressure is not scored: only 1 supporting span was found, below the minimum of 3, so it is reported as insufficient evidence.",
};

/* ---------------------------------------------------------------- quotes -- */

export const quotes = {
  eyebrow: "What people say",
  heading: "Three different jobs, three different reasons to trust it",
  items: [
    {
      quote:
        "“We were doing 40 phone screens a week and writing four sentences each. Now I read an evidence pack and disagree with it in writing. The disagreement is the point: I can see what it heard and say why I read it differently.”",
      initials: "PR",
      name: "Priya Raman",
      role: "Talent acquisition lead · Northwind Health System",
    },
    {
      quote:
        "“It asked me the question I’d been avoiding for six months, then showed me my own answer rewritten. I did the same session four times. The fourth one, I heard myself name a trade-off out loud for the first time.”",
      initials: "DO",
      name: "Daniel Okonkwo",
      role: "Backend engineer, 6 years · Manchester",
    },
    {
      quote:
        "“I approved it because of the boring parts: Frankfurt-only storage, SCIM deprovisioning that actually works, and an audit log that told me exactly who exported which candidate and when. No black box for me to defend.”",
      initials: "ES",
      name: "Elin Sørensen",
      role: "Head of IT security · Meridian Schools Trust",
    },
  ],
};

/* ------------------------------------------------------------------- faq -- */

export const faq = {
  eyebrow: "Questions we get asked first",
  heading: "The awkward ones, answered plainly",
  lead: "If the honest answer is “no, we don’t do that”, it says so.",
  items: [
    {
      id: "decisions",
      question: "Does Prepeet make hiring decisions?",
      answer:
        "No. Prepeet produces evidence: a transcript, tagged spans, competency signals with confidence ranges, and coverage flags. It does not advance, reject, rank or shortlist anyone. A named person makes every decision and their name is recorded against it. Tenants can require a second reviewer for any outcome, and screen candidates can request a human re-review of the interview.",
    },
    {
      id: "inference",
      question: "Can it detect personality, emotion, confidence or honesty?",
      answer:
        "No, and we will not build it. Prepeet does not analyse tone, pitch, accent, speaking rate, hesitation, facial expression or “cultural fit”, and it makes no prediction about how someone will perform in the job. It evaluates the content of what a candidate said against a written rubric, and nothing else. If a feature request needs an inference about the person rather than the answer, the answer is no.",
    },
    {
      id: "screen-mode",
      question: "What does a candidate actually see in screen mode?",
      answer:
        "The interview itself, a visible timer, live captions, and at the end a confirmation page saying the interview was submitted and roughly when they will hear back. That is all. No score, no band, no competency chart, no coaching, no recommendation and no reviewer note is generated or stored anywhere a screen candidate can reach. Coaching output is not merely hidden in screen mode: it is never produced. Practice mode is the opposite: candidates see everything about themselves.",
    },
    {
      id: "data",
      question: "Where is our data stored, and how long do you keep it?",
      answer:
        "Each tenant is pinned to a region at creation, EU (Frankfurt) or US (Virginia), and audio, transcripts and evaluations never leave it. Retention is set per tenant, from 30 days to 24 months; the default for screen interviews is 18 months, after which audio and transcripts are deleted and only the anonymised decision record remains. Candidates can request deletion of a practice session at any time from their account settings. Deletion is a real delete, including backups, within 35 days.",
    },
    {
      id: "accessibility",
      question: "Is it accessible? What if someone can’t use a microphone?",
      answer:
        "Prepeet targets WCAG 2.2 AA throughout. Live captions are always on by default and resizable, the whole interview is keyboard operable, push-to-talk replaces voice activity detection where that is easier, and extra time can be granted per invitation without the candidate having to disclose why. Where a spoken interview is not a reasonable format at all, the recruiter can withdraw the invitation and run the conversation themselves. A request for an adjustment is never recorded as a candidate outcome.",
    },
    {
      id: "connection",
      question: "What happens if the connection drops mid-interview?",
      answer:
        "The session pauses and the clock stops. Everything said up to that point is already committed, so nothing is lost. The candidate has ten minutes to rejoin using the same link, and the interviewer picks up with a short recap of where you were. If they cannot get back, the session is marked ended early and the recruiter sees a coverage warning rather than a low score: an interview that was cut short is reported as short, not as weak. Re-issuing the invitation takes one click.",
    },
    {
      id: "bias",
      question: "How do you manage bias?",
      answer:
        "Four ways, none of them a silver bullet. First, scope: only rubric-relevant content is evaluated, and speech characteristics are excluded by design. Second, anchors: every rubric level has written criteria and worked examples, so scoring is comparison against text rather than against an impression. Third, calibration: tenants run a fixed evidence set before go-live and again on every rubric or model change, and the drift report is visible to them, not just to us. Fourth, measurement: adverse-impact reporting by role and rubric version is available to tenant admins, and we publish our known limitations. Accented speech recognition accuracy and domain coverage outside our six maintained rubrics are the two we are least happy with today. A human still decides, which is the most important control of all.",
    },
    {
      id: "practice-history",
      question:
        "Can candidates use their practice history in a real application?",
      answer:
        "No, and this one is deliberate. Practice sessions live in the candidate’s own account and are never visible to any employer tenant, even if the candidate later interviews with that employer through Prepeet. There is no shared score, no portable rating and no way for a recruiter to see that someone practised. Practice is for the candidate, screening is for the employer, and the two data stores do not talk to each other.",
    },
  ],
};

/* ------------------------------------------------------------------- cta -- */

export const finalCta = {
  heading: "Start with one conversation",
  lead: "Practise free in your own account, or run a pilot requisition with your own rubric and see the evidence pack before you commit to anything.",
};

/* ---------------------------------------------------------------- footer -- */

export const footer = {
  blurb:
    "Realtime voice interviews with evidence you can read, quote and argue with. People decide; Prepeet shows its working.",
  columns: [
    {
      id: "foot-product",
      heading: "Product",
      links: [
        { label: "Features", href: "#product" },
        { label: "How it works", href: "#how" },
        { label: "Evidence model", href: "#evidence" },
        { label: "Use cases", href: "#use-cases" },
        { label: "Questions", href: "#faq" },
      ],
    },
    {
      id: "foot-candidates",
      heading: "For candidates",
      links: [
        { label: "Create an account", href: "/register" },
        { label: "Sign in", href: "/login" },
        { label: "Your practice sessions", href: "/practice" },
        { label: "Start an interview", href: "/practice/new" },
        { label: "Privacy and data controls", href: "/profile" },
      ],
    },
    {
      /** Every recruiter screen is unported, so this column answers in place rather than linking nowhere. */
      id: "foot-employers",
      heading: "For employers",
      links: [
        { label: "How screening works", href: "#how" },
        { label: "What a reviewer sees", href: "#evidence" },
        { label: "Rubrics and domains", href: "#use-cases" },
        { label: "Appeals and human review", href: "#faq" },
      ],
    },
    {
      id: "foot-company",
      heading: "Company & legal",
      links: [
        { label: "Data residency and retention", href: "#faq" },
        { label: "How we manage bias", href: "#faq" },
        { label: "Sign in", href: "/login" },
      ],
    },
  ] as { id: string; heading: string; links: NavLink[] }[],
  /** Not links. They are issued at contract and there is nothing to link to. */
  notices: [
    "Privacy notice: issued at contract",
    "Terms of service: issued at contract",
    "Sub-processor list: on request",
  ],
  copyright: "© 2026 Prepeet Ltd. Registered in England & Wales.",
  disclaimer:
    "Prepeet produces evidence for human reviewers. It does not make hiring decisions, and it does not infer personality, emotion or honesty.",
};
