"""The model-backed interviewer: Claude behind the Interviewer port (ADR-0019).

Briefed from the session's own pins - persona, role, plan, length - and
held to rules the prompt states and the code enforces: one question at a
time, no evaluation or feedback during the interview, no facts the
candidate did not state, and an end the model must signal explicitly.
The code adds what a prompt cannot promise: a hard cap on questions from
the plan and the configured length, and a measured latency on every turn.

The client is injected as one async callable so the loop is proven
against a fake; the real Anthropic client is constructed only from
configuration.
"""

from __future__ import annotations

import time
from collections.abc import Awaitable, Callable
from dataclasses import dataclass, field

from prepeet_ai.agent.ports import Turn
from prepeet_ai.agent.timeline import Brief

END_MARKER = "[END]"
"""What the model says when the interview is over: explicit, never inferred."""

DEFAULT_MODEL = "claude-sonnet-5"
MAX_QUESTION_TOKENS = 200

Complete = Callable[[str, list[dict[str, str]]], Awaitable[str]]
"""(system prompt, messages) -> the model's reply text."""


def system_prompt(brief: Brief) -> str:
    """The interviewer's standing instructions, from the brief."""
    competencies = ", ".join(brief.competencies) or "the role's core competencies"
    stages = brief.plan.get("stages")
    plan_line = ""
    if isinstance(stages, list):
        plan_line = "Follow this plan in order: " + ", ".join(str(s) for s in stages) + "."
    lines = (
        f"You are {brief.persona_name}, an interviewer. Style: {brief.persona_style}. "
        f"{brief.persona_description}",
        f"You are interviewing a candidate for {brief.role_title}. "
        f"The competencies to explore: {competencies}.",
        f"The interview is {brief.minutes} minutes long.",
        plan_line,
        "Rules, in order of importance:",
        "1. Ask exactly one question per turn, in at most two sentences.",
        "2. Never evaluate, score, praise or criticise an answer during the interview; "
        "acknowledge briefly and move on.",
        "3. Never state a fact about the candidate they did not say themselves.",
        "4. Prefer questions that invite a specific example with a measurable outcome.",
        f"5. When the plan is complete or time is up, reply with exactly {END_MARKER} "
        "and nothing else.",
    )
    return "\n".join(line for line in lines if line)


@dataclass
class ClaudeInterviewer:
    """Asks from the brief; the client decides the words, the code the limits."""

    brief: Brief
    complete: Complete
    max_questions: int = 0
    _history: list[dict[str, str]] = field(default_factory=list)
    _asked: int = 0

    def __post_init__(self) -> None:
        """Bound questions by the plan when the caller did not: two per stage, floor of three."""
        if self.max_questions <= 0:
            stages = self.brief.plan.get("stages")
            count = len(stages) if isinstance(stages, list) else 3
            self.max_questions = max(3, 2 * count)

    async def _ask(self, user_text: str) -> Turn | None:
        self._history.append({"role": "user", "content": user_text})
        started = time.monotonic()
        reply = (await self.complete(system_prompt(self.brief), list(self._history))).strip()
        latency_ms = int((time.monotonic() - started) * 1000)
        if reply.startswith(END_MARKER) or not reply:
            return None
        # A model that keeps talking past its one question gets cut to it:
        # the first paragraph is the question, the rest is not spoken.
        text = reply.split("\n\n", 1)[0].strip()
        self._history.append({"role": "assistant", "content": text})
        self._asked += 1
        return Turn(text=text, latency_ms=latency_ms)

    async def opening(self) -> Turn:
        """Greet and ask the first question."""
        turn = await self._ask(
            "The candidate has joined. Greet them briefly and ask your first question."
        )
        if turn is None:
            return Turn(
                text="Welcome. Tell me about a piece of work you led recently.", latency_ms=0
            )
        return turn

    async def next_question(self, candidate_said: str) -> Turn | None:
        """Continue from what the candidate said, or end when the cap or the model says so."""
        if self._asked >= self.max_questions:
            return None
        return await self._ask(candidate_said)


def anthropic_completer(api_key: str, model: str = DEFAULT_MODEL) -> Complete:  # pragma: no cover
    """The real client, built only from configuration."""
    from anthropic import AsyncAnthropic

    client = AsyncAnthropic(api_key=api_key)

    async def complete(system: str, messages: list[dict[str, str]]) -> str:
        response = await client.messages.create(
            model=model,
            max_tokens=MAX_QUESTION_TOKENS,
            system=system,
            messages=messages,  # type: ignore[arg-type]
        )
        from anthropic.types import TextBlock

        return "".join(block.text for block in response.content if isinstance(block, TextBlock))

    return complete
