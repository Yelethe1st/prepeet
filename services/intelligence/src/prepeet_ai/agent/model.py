"""The model-backed interviewer: any language model behind the port (ADR-0019).

Briefed from the session's own pins - persona, role, plan, length - and
held to rules the prompt states and the code enforces: one question at a
time, no evaluation or feedback during the interview, no facts the
candidate did not state, and an end the model must signal explicitly.
The code adds what a prompt cannot promise: a hard cap on questions from
the plan and the configured length, and a measured latency on every turn.

The provider is a deployment choice, never a code one. The interviewer
takes one async completion callable; completer_from_config builds it for
whichever provider the deployment named - Anthropic, or anything that
speaks the OpenAI-compatible chat API, which is OpenAI itself, Hugging
Face's router and inference endpoints, and every local open-weights
server (Ollama, vLLM, LM Studio, TGI). A local model needs no key and
transfers nothing, so ADR-0019's admissibility terms are met trivially.
"""

from __future__ import annotations

import time
from collections.abc import Awaitable, Callable, Mapping
from dataclasses import dataclass, field

from prepeet_ai.agent.ports import Turn
from prepeet_ai.agent.timeline import Brief

END_MARKER = "[END]"
"""What the model says when the interview is over: explicit, never inferred."""

MAX_QUESTION_TOKENS = 200

Complete = Callable[[str, list[dict[str, str]]], Awaitable[str]]
"""(system prompt, messages) -> the model's reply text."""

PROVIDERS = ("anthropic", "openai", "openai-compatible", "huggingface")
"""What PREPEET_LLM_PROVIDER may name. Three of the four share one adapter."""


@dataclass(frozen=True)
class ModelConfig:
    """The deployment's language model choice, all from the environment."""

    provider: str
    model: str
    api_key: str = ""
    base_url: str = ""

    @classmethod
    def from_env(cls, env: Mapping[str, str]) -> ModelConfig | None:
        """Read the choice; None when no provider is named (the scripted floor runs)."""
        provider = env.get("PREPEET_LLM_PROVIDER", "").strip().lower()
        if not provider:
            return None
        return cls(
            provider=provider,
            model=env.get("PREPEET_LLM_MODEL", "").strip(),
            api_key=env.get("PREPEET_LLM_API_KEY", "").strip(),
            base_url=env.get("PREPEET_LLM_BASE_URL", "").strip(),
        )

    @property
    def version(self) -> str:
        """The provenance string recorded on every turn: provider and model."""
        return f"{self.provider}:{self.model}"


class ModelConfigError(ValueError):
    """The deployment named a provider it did not configure completely."""


def validate(config: ModelConfig) -> ModelConfig:
    """Refuse an incomplete choice loudly rather than failing mid-interview.

    A cloud provider needs a key; a Hugging Face or plain OpenAI-compatible
    endpoint needs a base URL (a key only if the endpoint demands one);
    every provider needs a model name.
    """
    if config.provider not in PROVIDERS:
        raise ModelConfigError(
            f"PREPEET_LLM_PROVIDER={config.provider!r} is not one of {', '.join(PROVIDERS)}"
        )
    if not config.model:
        raise ModelConfigError("PREPEET_LLM_MODEL is required")
    if config.provider in ("anthropic", "openai") and not config.api_key:
        raise ModelConfigError(f"PREPEET_LLM_API_KEY is required for {config.provider}")
    if config.provider in ("openai-compatible", "huggingface") and not config.base_url:
        raise ModelConfigError(f"PREPEET_LLM_BASE_URL is required for {config.provider}")
    return config


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
class ModelInterviewer:
    """Asks from the brief; the model decides the words, the code the limits."""

    brief: Brief
    complete: Complete
    max_questions: int = 0
    # version is recorded on every turn boundary: which provider and model
    # asked this question, so a session's provenance names it.
    version: str = "scripted"
    _history: list[dict[str, str]] = field(default_factory=list)
    _asked: int = 0

    def __post_init__(self) -> None:
        """Bound questions by the plan when the caller did not: two per stage, floor of three.

        A retake is exactly one question, whatever the plan says.
        """
        if self.brief.redo_question:
            self.max_questions = 1
        elif self.max_questions <= 0:
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
        return Turn(text=text, latency_ms=latency_ms, model_version=self.version)

    async def opening(self) -> Turn:
        """Greet and ask the first question; for a retake, ask exactly the original question."""
        if self.brief.redo_question:
            self._asked += 1
            return Turn(
                text=f"Let us take that one again. {self.brief.redo_question}",
                latency_ms=0,
                model_version=self.version,
            )
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


def completer_from_config(config: ModelConfig) -> Complete:  # pragma: no cover - SDK glue
    """The real client for the named provider, built only from configuration."""
    validate(config)
    if config.provider == "anthropic":
        return _anthropic_completer(config)
    return _openai_compatible_completer(config)


def _anthropic_completer(config: ModelConfig) -> Complete:  # pragma: no cover - SDK glue
    from anthropic import AsyncAnthropic
    from anthropic.types import TextBlock

    client = AsyncAnthropic(api_key=config.api_key, base_url=config.base_url or None)

    async def complete(system: str, messages: list[dict[str, str]]) -> str:
        response = await client.messages.create(
            model=config.model,
            max_tokens=MAX_QUESTION_TOKENS,
            system=system,
            messages=messages,  # type: ignore[arg-type]
        )
        return "".join(block.text for block in response.content if isinstance(block, TextBlock))

    return complete


def _openai_compatible_completer(config: ModelConfig) -> Complete:  # pragma: no cover - SDK glue
    """OpenAI, Hugging Face, and every local server speaking the chat API."""
    from openai import AsyncOpenAI

    client = AsyncOpenAI(
        api_key=config.api_key or "not-needed",
        base_url=config.base_url or None,
    )

    async def complete(system: str, messages: list[dict[str, str]]) -> str:
        response = await client.chat.completions.create(
            model=config.model,
            max_tokens=MAX_QUESTION_TOKENS,
            messages=[{"role": "system", "content": system}, *messages],  # type: ignore[list-item]
        )
        choice = response.choices[0].message.content if response.choices else None
        return choice or ""

    return complete
