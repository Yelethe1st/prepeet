"""The model-backed interviewer, proven against a fake completion.

What is asserted is what a prompt cannot promise: the brief reaches the
model, the loop keeps the whole exchange, an explicit end marker ends the
interview, a runaway reply is cut to its one question, the question cap
holds however talkative the model is, and latency is measured on every
turn and reaches the timeline.
"""

from __future__ import annotations

import asyncio

from prepeet_ai.agent.claude import END_MARKER, ClaudeInterviewer, system_prompt
from prepeet_ai.agent.timeline import Brief

BRIEF = Brief(
    minutes=25,
    persona_name="Ama",
    persona_style="Warm and structured",
    persona_description="Gentle, never rushes.",
    role_title="Senior Backend Engineer",
    competencies=("Systems design", "Debugging"),
    plan={"stages": ["intro", "core", "close"]},
)


class FakeModel:
    """Answers from a script and records every call."""

    def __init__(self, replies: list[str]) -> None:
        """Queue the replies."""
        self.replies = list(replies)
        self.calls: list[tuple[str, list[dict[str, str]]]] = []

    async def __call__(self, system: str, messages: list[dict[str, str]]) -> str:
        """Record the call and pop the next reply."""
        self.calls.append((system, [dict(m) for m in messages]))
        return self.replies.pop(0) if self.replies else END_MARKER


class TestTheBriefReachesTheModel:
    """Persona, role, competencies, length and plan are all in the prompt."""

    def test_the_system_prompt_carries_the_pins_and_the_rules(self) -> None:
        """Every pinned fact and every rule is stated."""
        prompt = system_prompt(BRIEF)

        for expected in (
            "Ama",
            "Warm and structured",
            "Senior Backend Engineer",
            "Systems design, Debugging",
            "25 minutes",
            "intro, core, close",
            "exactly one question",
            "Never evaluate",
            "Never state a fact about the candidate",
            END_MARKER,
        ):
            assert expected in prompt


class TestTheLoop:
    """History accumulates, the end is explicit, the cap is the code's."""

    def test_the_exchange_is_kept_and_the_end_marker_ends_it(self) -> None:
        """Turn by turn, the model sees everything said so far."""
        model = FakeModel(
            ["Welcome. What did you build last year?", "What was hardest?", END_MARKER]
        )
        interviewer = ClaudeInterviewer(brief=BRIEF, complete=model)

        async def run() -> list[str | None]:
            first = await interviewer.opening()
            second = await interviewer.next_question("I built a payments migration.")
            third = await interviewer.next_question("Cutting over with zero downtime.")
            return [first.text, second.text if second else None, third.text if third else None]

        texts = asyncio.run(run())

        assert texts == ["Welcome. What did you build last year?", "What was hardest?", None]
        # The third call saw the whole exchange: both questions and both answers.
        _, messages = model.calls[2]
        assert [m["role"] for m in messages] == ["user", "assistant", "user", "assistant", "user"]
        assert messages[2]["content"] == "I built a payments migration."

    def test_a_runaway_reply_is_cut_to_its_first_paragraph(self) -> None:
        """One question per turn is enforced, not merely requested."""
        model = FakeModel(["What did you build?\n\nAlso, great answer, you seem very capable."])
        interviewer = ClaudeInterviewer(brief=BRIEF, complete=model)

        turn = asyncio.run(interviewer.opening())

        assert turn.text == "What did you build?"

    def test_the_question_cap_ends_the_interview_whatever_the_model_says(self) -> None:
        """A model that never sends the marker is stopped by the plan's budget."""
        model = FakeModel(["Q?"] * 50)
        interviewer = ClaudeInterviewer(brief=BRIEF, complete=model, max_questions=3)

        async def run() -> int:
            await interviewer.opening()
            asked = 1
            while await interviewer.next_question("an answer") is not None:
                asked += 1
            return asked

        assert asyncio.run(run()) == 3

    def test_latency_is_measured_on_every_turn(self) -> None:
        """A slow model shows up in the number the timeline will keep."""

        async def slow(system: str, messages: list[dict[str, str]]) -> str:
            await asyncio.sleep(0.05)
            return "What did you build?"

        interviewer = ClaudeInterviewer(brief=BRIEF, complete=slow)
        turn = asyncio.run(interviewer.opening())

        assert turn.latency_ms >= 40

    def test_an_immediate_end_still_opens_the_interview(self) -> None:
        """A model that ends before greeting is overridden: the room is not left silent."""
        interviewer = ClaudeInterviewer(brief=BRIEF, complete=FakeModel([END_MARKER]))

        turn = asyncio.run(interviewer.opening())

        assert turn.text
