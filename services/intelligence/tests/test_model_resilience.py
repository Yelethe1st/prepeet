"""What the interviewer does when the model does not answer.

A live interview is the one place where a provider failure has a person
sitting in the silence it makes. Neither failure mode was handled: no timeout
was set on either client, so both inherited the SDK default of roughly ten
minutes, and `_ask` awaited the completion bare, so a rate limit or a 500 on
question four propagated out of the conversation loop and ended the session.

The `try` in the worker only wraps *construction* of the interviewer, which
catches a bad key at startup and nothing at all afterwards.

These tests drive the interviewer with completions that hang and raise, which
is what a provider actually does, rather than asserting that a timeout argument
was passed to a client nobody builds in a test.
"""

from __future__ import annotations

import asyncio

import pytest

from prepeet_ai.agent.model import ModelConfig, ModelConfigError, ModelInterviewer, validate
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


def interviewer(complete, **kwargs) -> ModelInterviewer:
    """A model interviewer over the given completion."""
    return ModelInterviewer(brief=BRIEF, complete=complete, version="test:model", **kwargs)


class TestTheTimeout:
    """A provider that never answers must not become a candidate waiting."""

    async def _hang(self, system: str, messages: list[dict[str, str]]) -> str:
        """Never answer, the way a wedged connection never answers."""
        await asyncio.sleep(3600)
        return "never"

    def test_a_hanging_provider_does_not_hang_the_interview(self) -> None:
        """The interviewer gives up on its own budget, not the SDK's."""

        async def body() -> None:
            subject = interviewer(self._hang, timeout_seconds=0.05)

            # The assertion is the wait_for around the call: without a timeout of
            # its own the interviewer would sit here for the SDK's ten minutes,
            # and this test would fail by exceeding its own budget rather than by
            # asserting anything.
            turn = await asyncio.wait_for(subject.opening(), timeout=5)

            assert turn is not None
            assert "fallback" in turn.model_version

        asyncio.run(body())

    def test_the_timeout_is_bounded_by_default(self) -> None:
        """Not the SDK's default, which is long enough to lose the candidate."""

        async def body() -> None:
            assert 0 < ModelInterviewer(brief=BRIEF, complete=self._hang).timeout_seconds <= 60

        asyncio.run(body())

    def test_the_timeout_is_read_from_the_environment(self) -> None:
        """A deployment can shorten it without a code change."""
        config = ModelConfig.from_env(
            {
                "PREPEET_LLM_PROVIDER": "anthropic",
                "PREPEET_LLM_MODEL": "claude-sonnet-5",
                "PREPEET_LLM_API_KEY": "sk-test",
                "PREPEET_LLM_TIMEOUT_SECONDS": "12.5",
            }
        )
        assert config is not None
        assert config.timeout_seconds == 12.5

    @pytest.mark.parametrize("value", ["0", "-1", "not-a-number"])
    def test_an_unusable_timeout_is_refused_at_startup(self, value: str) -> None:
        """A zero or unparseable budget fails at startup, not mid-interview."""
        config = ModelConfig.from_env(
            {
                "PREPEET_LLM_PROVIDER": "anthropic",
                "PREPEET_LLM_MODEL": "claude-sonnet-5",
                "PREPEET_LLM_API_KEY": "sk-test",
                "PREPEET_LLM_TIMEOUT_SECONDS": value,
            }
        )
        assert config is not None
        with pytest.raises(ModelConfigError, match="TIMEOUT"):
            validate(config)


class Flaky:
    """Raises for the first `failures` calls, then answers."""

    def __init__(self, failures: int, reply: str = "What did you own on that?") -> None:
        """Fail the first `failures` calls, then answer with `reply`."""
        self.remaining = failures
        self.reply = reply
        self.calls = 0

    async def __call__(self, system: str, messages: list[dict[str, str]]) -> str:
        """Raise while failures remain, then answer."""
        self.calls += 1
        if self.remaining > 0:
            self.remaining -= 1
            raise RuntimeError("429 rate limited")
        return self.reply


class TestAFailingProvider:
    """A provider fault is a degraded interview, not an ended one."""

    def test_one_failure_does_not_end_the_interview(self) -> None:
        """A transient 429 used to propagate out of the conversation loop."""

        async def body() -> None:
            model = Flaky(failures=1)
            subject = interviewer(model)

            # Previously this raised straight through conversation.py and the
            # candidate's session ended on a transient 429.
            turn = await subject.opening()

            assert turn is not None
            assert turn.text

        asyncio.run(body())

    def test_a_fallback_turn_does_not_claim_the_model_asked_it(self) -> None:
        """Provenance is recorded per turn, so it has to stay true."""

        async def body() -> None:
            subject = interviewer(Flaky(failures=1))

            turn = await subject.opening()

            assert turn is not None
            assert turn.model_version == "test:model/fallback"

        asyncio.run(body())

    def test_the_interview_continues_after_a_failed_turn(self) -> None:
        """The next turn is the model's again, not a second fallback."""

        async def body() -> None:
            model = Flaky(failures=1)
            subject = interviewer(model)

            await subject.opening()
            following = await subject.next_question("I led the migration.")

            assert following is not None
            assert following.text == model.reply
            assert following.model_version == "test:model"

        asyncio.run(body())

    def test_a_provider_that_never_recovers_ends_the_interview(self) -> None:
        """A fallback loop would keep a candidate answering nothing forever."""

        async def body() -> None:
            subject = interviewer(Flaky(failures=99))

            await subject.opening()
            outcomes = [await subject.next_question(f"answer {i}") for i in range(5)]

            assert None in outcomes, "the interview never ended"

        asyncio.run(body())

    def test_a_recovery_resets_the_patience(self) -> None:
        """One failure now and one much later is not a broken provider."""

        async def body() -> None:
            replies = iter(
                [RuntimeError("500"), "Question two?", RuntimeError("500"), "Question four?"]
            )

            async def flapping(system: str, messages: list[dict[str, str]]) -> str:
                value = next(replies)
                if isinstance(value, Exception):
                    raise value
                return value

            subject = interviewer(flapping, max_questions=10)

            assert await subject.opening() is not None
            assert await subject.next_question("a") is not None
            assert await subject.next_question("b") is not None
            assert await subject.next_question("c") is not None

        asyncio.run(body())

    def test_cancellation_is_not_swallowed(self) -> None:
        """Shutting the worker down must not be mistaken for a provider fault."""

        async def body() -> None:

            async def cancelled(system: str, messages: list[dict[str, str]]) -> str:
                raise asyncio.CancelledError

            with pytest.raises(asyncio.CancelledError):
                await interviewer(cancelled).opening()

        asyncio.run(body())

    def test_the_question_cap_still_holds_across_fallbacks(self) -> None:
        """A fallback that did not count would let the interview run past its plan."""

        async def body() -> None:
            subject = interviewer(Flaky(failures=99), max_questions=2)

            turns = []
            for index in range(6):
                turn = await subject.opening() if index == 0 else await subject.next_question("x")
                turns.append(turn)
                if turn is None:
                    break

            assert len([t for t in turns if t is not None]) <= 2

        asyncio.run(body())
