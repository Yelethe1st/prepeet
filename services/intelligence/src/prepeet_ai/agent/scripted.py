"""The scripted interviewer: the walking skeleton's voice.

Asks a fixed sequence and stops. It exists so the whole pipe - room, speech,
timeline, egress, evaluation - is proven end to end before a model sits
here; the model-backed interviewer (DEC-10, Claude) replaces it behind the
same port.
"""

from __future__ import annotations

from collections.abc import Sequence

from prepeet_ai.agent.ports import Turn


class ScriptedInterviewer:
    """Asks the given questions in order, then ends the interview."""

    def __init__(self, opening: str, questions: Sequence[str]) -> None:
        """Remember the opening line and the questions to ask after it."""
        self._opening = opening
        self._questions = list(questions)
        self._asked = 0

    async def opening(self) -> Turn:
        """The first thing said in the room."""
        return Turn(text=self._opening, latency_ms=0)

    async def next_question(self, candidate_said: str) -> Turn | None:
        """The next scripted question, ignoring the answer; None at the end."""
        if self._asked >= len(self._questions):
            return None
        question = self._questions[self._asked]
        self._asked += 1
        return Turn(text=question, latency_ms=0)
