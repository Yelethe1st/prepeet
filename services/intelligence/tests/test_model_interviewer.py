"""The model-backed interviewer, proven against a fake completion.

What is asserted is what a prompt cannot promise: the brief reaches the
model, the loop keeps the whole exchange, an explicit end marker ends the
interview, a runaway reply is cut to its one question, the question cap
holds however talkative the model is, and latency is measured on every
turn and reaches the timeline.
"""

from __future__ import annotations

import asyncio

from prepeet_ai.agent.model import (
    END_MARKER,
    ModelConfig,
    ModelConfigError,
    ModelInterviewer,
    system_prompt,
    validate,
)
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
        interviewer = ModelInterviewer(brief=BRIEF, complete=model)

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
        interviewer = ModelInterviewer(brief=BRIEF, complete=model)

        turn = asyncio.run(interviewer.opening())

        assert turn.text == "What did you build?"

    def test_the_question_cap_ends_the_interview_whatever_the_model_says(self) -> None:
        """A model that never sends the marker is stopped by the plan's budget."""
        model = FakeModel(["Q?"] * 50)
        interviewer = ModelInterviewer(brief=BRIEF, complete=model, max_questions=3)

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

        interviewer = ModelInterviewer(brief=BRIEF, complete=slow)
        turn = asyncio.run(interviewer.opening())

        assert turn.latency_ms >= 40

    def test_an_immediate_end_still_opens_the_interview(self) -> None:
        """A model that ends before greeting is overridden: the room is not left silent."""
        interviewer = ModelInterviewer(brief=BRIEF, complete=FakeModel([END_MARKER]))

        turn = asyncio.run(interviewer.opening())

        assert turn.text


class TestTheProviderIsADeploymentChoice:
    """Any provider, named in the environment, refused loudly when incomplete."""

    def test_no_provider_means_the_scripted_floor(self) -> None:
        """An empty environment names nothing, so nothing model-backed runs."""
        assert ModelConfig.from_env({}) is None

    def test_each_provider_family_is_admitted_with_what_it_needs(self) -> None:
        """Cloud needs a key; local and hosted-compatible need a base URL; all need a model."""
        cases = {
            "anthropic": {"PREPEET_LLM_MODEL": "claude-sonnet-5", "PREPEET_LLM_API_KEY": "k"},
            "openai": {"PREPEET_LLM_MODEL": "gpt-5", "PREPEET_LLM_API_KEY": "k"},
            "openai-compatible": {
                "PREPEET_LLM_MODEL": "llama3.1:8b",
                "PREPEET_LLM_BASE_URL": "http://localhost:11434/v1",
            },
            "huggingface": {
                "PREPEET_LLM_MODEL": "meta-llama/Llama-3.1-8B-Instruct",
                "PREPEET_LLM_BASE_URL": "https://router.huggingface.co/v1",
                "PREPEET_LLM_API_KEY": "hf_k",
            },
        }
        for provider, env in cases.items():
            config = ModelConfig.from_env({"PREPEET_LLM_PROVIDER": provider, **env})
            assert config is not None
            assert validate(config).version.startswith(provider + ":")

    def test_a_local_open_weights_server_needs_no_key(self) -> None:
        """Nothing leaves the machine, so there is nothing to authorise."""
        config = ModelConfig.from_env(
            {
                "PREPEET_LLM_PROVIDER": "openai-compatible",
                "PREPEET_LLM_MODEL": "qwen2.5:7b",
                "PREPEET_LLM_BASE_URL": "http://localhost:11434/v1",
            }
        )
        assert config is not None
        assert validate(config).api_key == ""

    def test_incomplete_choices_are_refused_by_name(self) -> None:
        """Failing at start beats failing mid-interview."""
        refused = [
            {"PREPEET_LLM_PROVIDER": "anthropic", "PREPEET_LLM_MODEL": "claude-sonnet-5"},
            {"PREPEET_LLM_PROVIDER": "openai-compatible", "PREPEET_LLM_MODEL": "llama3"},
            {"PREPEET_LLM_PROVIDER": "openai", "PREPEET_LLM_API_KEY": "k"},
            {"PREPEET_LLM_PROVIDER": "bard", "PREPEET_LLM_MODEL": "x", "PREPEET_LLM_API_KEY": "k"},
        ]
        for env in refused:
            config = ModelConfig.from_env(env)
            assert config is not None
            try:
                validate(config)
            except ModelConfigError:
                continue
            raise AssertionError(f"{env} was admitted")

    def test_the_turn_names_the_provider_and_model_that_asked(self) -> None:
        """Provenance: which model asked rides every turn boundary."""
        interviewer = ModelInterviewer(
            brief=BRIEF,
            complete=FakeModel(["What did you build?"]),
            version="openai-compatible:llama3.1:8b",
        )
        turn = asyncio.run(interviewer.opening())
        assert turn.model_version == "openai-compatible:llama3.1:8b"


class TestARetakeAsksExactlyTheOriginalQuestion:
    """PRC-03: one question, the original, then the interview ends."""

    def test_the_redo_question_is_asked_verbatim_and_nothing_follows(self) -> None:
        """The model is not consulted for the question and gets no second turn."""
        import dataclasses

        brief = dataclasses.replace(BRIEF, redo_question="Tell me about a migration you led.")
        model = FakeModel(["Some other question?"] * 3)
        interviewer = ModelInterviewer(brief=brief, complete=model)

        async def run() -> tuple[str, object]:
            first = await interviewer.opening()
            second = await interviewer.next_question("I led the payments migration.")
            return first.text, second

        first, second = asyncio.run(run())

        assert "Tell me about a migration you led." in first
        assert second is None
        assert model.calls == []
