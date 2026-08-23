/* Prepeet · data.js
   Seeded mock data shared across screens. Deterministic — no randomness, no fetch.
   Stable IDs are used by every dynamic route so any screen can be opened directly.
*/
(function () {
  var P = window.Prepeet = window.Prepeet || {};

  P.ids = {
    session: 'ses_7Kq2XA',          // canonical practice session used in /candidate/session/[id]/*
    screenSession: 'ses_4Bz9QD',    // canonical screen-mode session
    candidate: 'cand_R4T9MP',
    invitation: 'inv_QX72HL',
    tenant: 'tn_northwind',
    role: 'role_icu_rn_2026'
  };

  P.data = {
    candidateUser: {
      name: 'Daniel Okonkwo', initials: 'DO', role: 'Candidate',
      email: 'daniel.okonkwo@example.com',
      headline: 'Backend engineer · 6 years · Manchester, UK',
      target: 'Senior Backend Engineer',
      timezone: 'Europe/London'
    },
    adminUser: { name: 'Priya Raman', initials: 'PR', email: 'priya.raman@northwindhealth.example', role: 'Talent acquisition lead' },
    tenant: {
      id: 'tn_northwind', name: 'Northwind Health System', slug: 'northwind-health',
      plan: 'Enterprise', seats: 48, region: 'EU (Frankfurt)', retention: '18 months'
    },

    /* ------------------------------------------------------------- personas */
    personas: [
      { id: 'per_ama', name: 'Ama', style: 'Warm and structured', voice: 'Mid-tone, unhurried',
        desc: 'Opens with context, gives you a moment to think, and follows up gently when an answer is thin.',
        best: 'First sessions, nerves, behavioural rounds', initials: 'AM' },
      { id: 'per_ravi', name: 'Ravi', style: 'Direct and technical', voice: 'Brisk, precise',
        desc: 'Moves quickly, probes trade-offs and asks for specifics. Interrupts less politely than Ama.',
        best: 'Systems design, technical depth, senior loops', initials: 'RA' },
      { id: 'per_lena', name: 'Lena', style: 'Panel chair', voice: 'Formal, measured',
        desc: 'Runs a competency-based panel format with scripted follow-ups and time boxing per question.',
        best: 'Public sector, clinical and education panels', initials: 'LE' },
      { id: 'per_marcus', name: 'Marcus', style: 'Sceptical stakeholder', voice: 'Low, deliberate',
        desc: 'Pushes back on claims and asks you to defend decisions with evidence. Harder than a real first round.',
        best: 'Stakeholder management, sales, exec practice', initials: 'MA' }
    ],

    /* --------------------------------------------------------- interview shapes */
    shapes: [
      { id: 'shape_behavioural', name: 'Behavioural', icon: 'messages-square', desc: 'Competency questions with STAR-style follow-ups on real examples from your history.', minutes: [15, 25, 40] },
      { id: 'shape_technical', name: 'Technical deep-dive', icon: 'code-2', desc: 'Verbal reasoning about systems, trade-offs and failure modes. No coding editor.', minutes: [25, 40] },
      { id: 'shape_case', name: 'Case / scenario', icon: 'clipboard-list', desc: 'A realistic scenario from the role, worked through out loud with escalating constraints.', minutes: [25, 40] },
      { id: 'shape_screen', name: 'Recruiter screen', icon: 'phone-call', desc: 'Motivation, availability, salary expectations and a light skills pass.', minutes: [15, 25] },
      { id: 'shape_panel', name: 'Panel simulation', icon: 'users-round', desc: 'Three rotating interviewer viewpoints in one session, as used in clinical and education hiring.', minutes: [40, 60] }
    ],

    /* ---------------------------------------------------------- role library */
    roleLibrary: [
      { id: 'rl_swe', domain: 'Software engineering', title: 'Senior Backend Engineer', org: 'Product company, 200–800 people', comps: ['Systems design', 'Debugging & incident response', 'Collaboration', 'Technical communication'] },
      { id: 'rl_em', domain: 'Software engineering', title: 'Engineering Manager', org: 'Scale-up', comps: ['People development', 'Delivery management', 'Technical judgement', 'Conflict handling'] },
      { id: 'rl_pm', domain: 'Product management', title: 'Senior Product Manager', org: 'B2B SaaS', comps: ['Product sense', 'Prioritisation', 'Stakeholder management', 'Data reasoning'] },
      { id: 'rl_rn', domain: 'Nursing', title: 'Registered Nurse — Intensive Care', org: 'NHS-style trust or health system', comps: ['Clinical reasoning', 'Patient safety', 'Escalation & handover', 'Compassionate communication'] },
      { id: 'rl_cne', domain: 'Nursing', title: 'Clinical Nurse Educator', org: 'Teaching hospital', comps: ['Teaching & assessment', 'Clinical currency', 'Feedback delivery', 'Programme design'] },
      { id: 'rl_teach', domain: 'Teaching', title: 'Secondary Mathematics Teacher', org: 'Multi-academy trust', comps: ['Subject pedagogy', 'Behaviour management', 'Assessment & feedback', 'Safeguarding awareness'] },
      { id: 'rl_sales', domain: 'Sales', title: 'Enterprise Account Executive', org: 'Health-tech vendor', comps: ['Discovery', 'Commercial qualification', 'Objection handling', 'Forecast discipline'] },
      { id: 'rl_fin', domain: 'Finance', title: 'Financial Analyst — FP&A', org: 'Multi-site provider group', comps: ['Financial modelling', 'Variance analysis', 'Business partnering', 'Controls awareness'] }
    ],

    /* ------------------------------------------------- candidate competencies */
    competencies: [
      { id: 'systems-design', name: 'Systems design', score: 78, prev: 71, band: 'solid', ci: [70, 84], evidence: 6, note: 'Strong on data modelling; less consistent on failure modes.' },
      { id: 'technical-communication', name: 'Technical communication', score: 84, prev: 79, band: 'strong', ci: [79, 88], evidence: 9, note: 'Clear structure, good use of concrete numbers.' },
      { id: 'debugging', name: 'Debugging & incident response', score: 66, prev: 62, band: 'solid', ci: [56, 74], evidence: 4, note: 'Describes the fix more often than the diagnosis.' },
      { id: 'collaboration', name: 'Collaboration & influence', score: 72, prev: 74, band: 'solid', ci: [64, 79], evidence: 7, note: 'Good conflict example; ownership language sometimes vague.' },
      { id: 'ownership', name: 'Ownership & follow-through', score: 58, prev: 51, band: 'developing', ci: [47, 68], evidence: 3, note: 'Only three examples so far — needs more evidence.' },
      { id: 'prioritisation', name: 'Prioritisation under pressure', score: null, prev: null, band: 'insufficient', ci: null, evidence: 1, note: 'Not enough evidence to score yet. Try a case-format session.' }
    ],

    /* ------------------------------------------------------- candidate sessions */
    sessions: [
      { id: 'ses_7Kq2XA', title: 'Systems design deep-dive', role: 'Senior Backend Engineer', mode: 'practice', shape: 'Technical deep-dive', persona: 'Ravi', date: '2026-08-21', time: '18:40', minutes: 38, status: 'complete', score: 74, band: 'solid', questions: 6 },
      { id: 'ses_3Mn8PB', title: 'Behavioural — conflict & ownership', role: 'Senior Backend Engineer', mode: 'practice', shape: 'Behavioural', persona: 'Ama', date: '2026-08-18', time: '07:15', minutes: 24, status: 'complete', score: 69, band: 'solid', questions: 5 },
      { id: 'ses_4Bz9QD', title: 'ICU screening — Northwind Health', role: 'Registered Nurse — ICU', mode: 'screen', shape: 'Panel simulation', persona: 'Lena', date: '2026-08-16', time: '11:00', minutes: 41, status: 'submitted', score: null, band: null, questions: 8 },
      { id: 'ses_9Vt4LC', title: 'Engineering Manager — first pass', role: 'Engineering Manager', mode: 'practice', shape: 'Behavioural', persona: 'Ama', date: '2026-08-12', time: '20:05', minutes: 26, status: 'complete', score: 61, band: 'developing', questions: 5 },
      { id: 'ses_2Hd6RE', title: 'Incident response scenario', role: 'Senior Backend Engineer', mode: 'practice', shape: 'Case / scenario', persona: 'Marcus', date: '2026-08-09', time: '19:30', minutes: 31, status: 'complete', score: 66, band: 'solid', questions: 4 },
      { id: 'ses_5Wp1YF', title: 'Systems design — caching & consistency', role: 'Senior Backend Engineer', mode: 'practice', shape: 'Technical deep-dive', persona: 'Ravi', date: '2026-08-04', time: '18:00', minutes: 12, status: 'abandoned', score: null, band: null, questions: 2 },
      { id: 'ses_8Jc3TG', title: 'Recruiter screen practice', role: 'Senior Backend Engineer', mode: 'practice', shape: 'Recruiter screen', persona: 'Ama', date: '2026-07-30', time: '12:20', minutes: 16, status: 'complete', score: 71, band: 'solid', questions: 4 },
      { id: 'ses_6Ly0NH', title: 'Systems design deep-dive', role: 'Senior Backend Engineer', mode: 'practice', shape: 'Technical deep-dive', persona: 'Ravi', date: '2026-07-24', time: '18:45', minutes: 35, status: 'complete', score: 63, band: 'developing', questions: 6 },
      { id: 'ses_1Ab5VJ', title: 'Behavioural — stakeholder pushback', role: 'Senior Backend Engineer', mode: 'practice', shape: 'Behavioural', persona: 'Marcus', date: '2026-07-19', time: '08:30', minutes: 22, status: 'complete', score: 58, band: 'developing', questions: 5 },
      { id: 'ses_0Fg7WK', title: 'Systems design — event pipelines', role: 'Senior Backend Engineer', mode: 'practice', shape: 'Technical deep-dive', persona: 'Ravi', date: '2026-07-11', time: '19:10', minutes: 33, status: 'complete', score: 57, band: 'developing', questions: 6 },
      { id: 'ses_QpendA', title: 'Behavioural — feedback & growth', role: 'Engineering Manager', mode: 'practice', shape: 'Behavioural', persona: 'Ama', date: '2026-08-22', time: '21:12', minutes: 19, status: 'processing', score: null, band: null, questions: 5 },
      { id: 'ses_DraftB', title: 'Case — cost reduction scenario', role: 'Senior Backend Engineer', mode: 'practice', shape: 'Case / scenario', persona: 'Marcus', date: '2026-08-23', time: '09:02', minutes: 0, status: 'saved', score: null, band: null, questions: 0 }
    ],

    /* ------------------------------------------------------------- transcript */
    transcript: [
      { t: '00:00', who: 'ai', speaker: 'Ravi', text: 'Thanks for making the time, Daniel. I want to spend most of today on one system. Imagine we run appointment booking for a health network — about four million patients, spiky load on Monday mornings. Where would you start?' },
      { t: '00:24', who: 'user', speaker: 'You', text: 'I would start with the read and write shapes rather than the components. Booking is write-heavy in short bursts but read-heavy all day, so I would separate the availability read path from the reservation write path early.', spans: [{ kind: 'supporting', from: 'I would start with the read and write shapes', to: '' }] },
      { t: '00:58', who: 'ai', speaker: 'Ravi', text: 'Good. Say Monday at 08:00 you get thirty thousand booking attempts a minute against overlapping slots. How do you stop double booking?' },
      { t: '01:12', who: 'user', speaker: 'You', text: 'A conditional write on the slot row with an optimistic version, so two writers cannot both take the last slot. At the last place I worked we did this with a unique constraint on slot_id plus status, and let the loser retry against a refreshed availability read. It cut our double-booking incidents from roughly one a week to none in six months.' },
      { t: '02:05', who: 'ai', speaker: 'Ravi', text: 'What happens when the database that holds that constraint is the thing under pressure?' },
      { t: '02:14', who: 'user', speaker: 'You', text: 'Then I would shard by clinic, because bookings never cross clinics. Each shard owns its slots so the constraint stays local.' },
      { t: '02:41', who: 'ai', speaker: 'Ravi', text: 'And if a clinic is itself the hotspot?' },
      { t: '02:47', who: 'user', speaker: 'You', text: 'Probably a queue in front of it. I would let requests line up and process them in order.', spans: [{ kind: 'gap' }] },
      { t: '03:20', who: 'ai', speaker: 'Ravi', text: 'Talk me through what the patient sees while they are in that queue.' },
      { t: '03:26', who: 'user', speaker: 'You', text: 'Honestly, I have not designed that part before. I would want the UI to hold a soft reservation with a visible countdown rather than spin, so the patient knows the slot is theirs for two minutes. I would need to check what our booking partners support before committing to that.', spans: [{ kind: 'claim' }] },
      { t: '04:10', who: 'ai', speaker: 'Ravi', text: 'That is a fair answer. Let us switch to failure. The reservation service is healthy but the downstream EHR write is timing out. What do you do?' },
      { t: '04:22', who: 'user', speaker: 'You', text: 'I would fail closed — reject the booking rather than confirm something the clinical record does not know about. Then queue a repair job and alert. Confirming a booking the EHR never received is the worse outcome for a patient.', spans: [{ kind: 'supporting' }] },
      { t: '05:16', who: 'ai', speaker: 'Ravi', text: 'Earlier you said you shard by clinic. A moment ago you described a global queue. Which is it?' },
      { t: '05:24', who: 'user', speaker: 'You', text: 'Per-shard queue, not global. I said it loosely the first time.', spans: [{ kind: 'contradiction' }] }
    ],

    /* --------------------------------------------------------- recruiter roster */
    roster: [
      { id: 'cand_R4T9MP', name: 'Amara Osei', role: 'Registered Nurse — ICU', domain: 'Nursing', score: 82, band: 'strong', confidence: 'high', coverage: 92, status: 'pending', submitted: '2026-08-22 09:14', session: 'ses_4Bz9QD', reviewer: null, flags: [] },
      { id: 'cand_L7X2QN', name: 'Tomás Herrera', role: 'Registered Nurse — ICU', domain: 'Nursing', score: 74, band: 'solid', confidence: 'medium', coverage: 78, status: 'pending', submitted: '2026-08-22 08:02', session: 'ses_TH01', reviewer: null, flags: ['appeal'] },
      { id: 'cand_B2K8WD', name: 'Chloe Whitfield', role: 'Financial Analyst — FP&A', domain: 'Finance', score: 69, band: 'solid', confidence: 'medium', coverage: 71, status: 'pending', submitted: '2026-08-21 17:45', session: 'ses_CW01', reviewer: null, flags: [] },
      { id: 'cand_M9P3ZR', name: 'Ifeoma Adeyemi', role: 'Product Manager — Patient Apps', domain: 'Product management', score: 88, band: 'strong', confidence: 'high', coverage: 95, status: 'advanced', submitted: '2026-08-21 14:20', session: 'ses_IA01', reviewer: 'Priya Raman', flags: [] },
      { id: 'cand_V5N1HT', name: 'Daniel Okonkwo', role: 'Software Engineer — Integrations', domain: 'Software engineering', score: 74, band: 'solid', confidence: 'medium', coverage: 80, status: 'pending', submitted: '2026-08-21 11:05', session: 'ses_7Kq2XA', reviewer: null, flags: [] },
      { id: 'cand_C8R6JY', name: 'Sofia Marchetti', role: 'Enterprise Account Executive', domain: 'Sales', score: 63, band: 'developing', confidence: 'medium', coverage: 68, status: 'hold', submitted: '2026-08-20 16:30', session: 'ses_SM01', reviewer: 'Jonas Brandt', flags: [] },
      { id: 'cand_T3W7KL', name: 'Mateusz Zielinski', role: 'Registered Nurse — ICU', domain: 'Nursing', score: null, band: 'insufficient', confidence: 'low', coverage: 34, status: 'insufficient', submitted: '2026-08-20 10:12', session: 'ses_MZ01', reviewer: null, flags: ['short-session'] },
      { id: 'cand_D6Y4FQ', name: 'Grace Mbeki', role: 'Clinical Nurse Educator', domain: 'Nursing', score: 79, band: 'solid', confidence: 'high', coverage: 88, status: 'advanced', submitted: '2026-08-19 13:55', session: 'ses_GM01', reviewer: 'Priya Raman', flags: [] },
      { id: 'cand_H1S5BV', name: 'Oliver Nakamura', role: 'Financial Analyst — FP&A', domain: 'Finance', score: 55, band: 'developing', confidence: 'high', coverage: 84, status: 'declined', submitted: '2026-08-19 09:40', session: 'ses_ON01', reviewer: 'Jonas Brandt', flags: [] },
      { id: 'cand_K4Z9GX', name: 'Yusuf Demir', role: 'Enterprise Account Executive', domain: 'Sales', score: 77, band: 'solid', confidence: 'medium', coverage: 82, status: 'pending', submitted: '2026-08-18 15:10', session: 'ses_YD01', reviewer: null, flags: [] },
      { id: 'cand_N7Q2CE', name: 'Priyanka Shah', role: 'Product Manager — Patient Apps', domain: 'Product management', score: 71, band: 'solid', confidence: 'low', coverage: 58, status: 'pending', submitted: '2026-08-18 08:25', session: 'ses_PS01', reviewer: null, flags: ['low-confidence'] },
      { id: 'cand_A2M8DU', name: 'Liam Ferguson', role: 'Software Engineer — Integrations', domain: 'Software engineering', score: 68, band: 'solid', confidence: 'medium', coverage: 76, status: 'advanced', submitted: '2026-08-17 12:00', session: 'ses_LF01', reviewer: 'Priya Raman', flags: [] }
    ],

    statusMeta: {
      pending:      { label: 'Awaiting review', badge: 'badge-warning', icon: 'clock' },
      advanced:     { label: 'Advanced', badge: 'badge-success', icon: 'check-circle-2' },
      hold:         { label: 'On hold', badge: 'badge-neutral', icon: 'pause-circle' },
      declined:     { label: 'Not progressing', badge: 'badge-danger', icon: 'x-circle' },
      insufficient: { label: 'Insufficient evidence', badge: 'badge-outline', icon: 'help-circle' },
      complete:     { label: 'Reviewed', badge: 'badge-primary', icon: 'file-check' },
      submitted:    { label: 'Submitted', badge: 'badge-primary', icon: 'send' },
      processing:   { label: 'Processing', badge: 'badge-info', icon: 'loader' },
      abandoned:    { label: 'Ended early', badge: 'badge-neutral', icon: 'circle-slash' },
      saved:        { label: 'Saved progress', badge: 'badge-outline', icon: 'bookmark' }
    },

    /* --------------------------------------------------------------- tenants */
    tenants: [
      { id: 'tn_northwind', name: 'Northwind Health System', domain: 'Healthcare', sessions30: 1284, seats: 48, plan: 'Enterprise', spend: 2140.55, status: 'healthy', quota: 72 },
      { id: 'tn_orbital', name: 'Orbital Labs', domain: 'Software', sessions30: 964, seats: 32, plan: 'Growth', spend: 1477.10, status: 'healthy', quota: 61 },
      { id: 'tn_meridian', name: 'Meridian Schools Trust', domain: 'Education', sessions30: 612, seats: 24, plan: 'Growth', spend: 903.28, status: 'degraded', quota: 94 },
      { id: 'tn_caldera', name: 'Caldera Capital', domain: 'Finance', sessions30: 388, seats: 12, plan: 'Team', spend: 604.72, status: 'healthy', quota: 44 },
      { id: 'tn_brightpath', name: 'Brightpath Commercial', domain: 'Sales', sessions30: 341, seats: 16, plan: 'Team', spend: 512.90, status: 'healthy', quota: 38 },
      { id: 'tn_selfserve', name: 'Prepeet Practice (self-serve)', domain: 'Consumer', sessions30: 5412, seats: 0, plan: 'Practice', spend: 3980.14, status: 'healthy', quota: 55 }
    ],

    /* ------------------------------------------------------- helper functions */
  };

  /* Utility helpers used by pages */
  P.fmt = {
    date: function (iso) {
      var d = iso.split('-');
      var months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
      return Number(d[2]) + ' ' + months[Number(d[1]) - 1] + ' ' + d[0];
    },
    money: function (n) { return '$' + n.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 }); },
    num: function (n) { return n.toLocaleString('en-US'); }
  };

  P.confidenceMeta = {
    high:   { label: 'High confidence', desc: 'Multiple independent evidence spans support each competency score.' },
    medium: { label: 'Medium confidence', desc: 'Enough evidence for a provisional view; some competencies rest on a single answer.' },
    low:    { label: 'Low confidence', desc: 'Sparse evidence. Treat scores as indicative only and gather more in a live conversation.' }
  };
})();
