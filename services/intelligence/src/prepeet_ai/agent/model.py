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

import asyncio
import logging
import time
from collections.abc import Awaitable, Callable, Mapping
from dataclasses import dataclass, field

from prepeet_ai.agent.ports import Turn
from prepeet_ai.agent.timeline import Brief

logger = logging.getLogger(__name__)

END_MARKER = "[END]"
"""What the model says when the interview is over: explicit, never inferred."""

MAX_QUESTION_TOKENS = 200

FALLBACK_QUESTIONS = (
    "Tell me about a piece of work you are proud of.",
    "What was the hardest decision in that work, and what did you weigh?",
    "What would you do differently next time?",
    "What did you learn that you have carried into later work?",
)
"""Asked when the provider does not answer. Open enough to follow anything."""

DEFAULT_TIMEOUT_SECONDS = 20.0
"""One question's budget. See ModelConfig.timeout_seconds for why it is short."""


def _parse_timeout(raw: str) -> float:
    """Read the configured budget, leaving an unusable one to fail in validate.

    A value that cannot be parsed becomes a sentinel rather than the default,
    because falling back to the default would silently ignore what a deployment
    asked for.
    """
    if not raw.strip():
        return DEFAULT_TIMEOUT_SECONDS
    try:
        return float(raw)
    except ValueError:
        return float("nan")


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
    # How long one question may take. Both SDKs default to roughly ten
    # minutes, which is a sensible batch default and a catastrophic one for a
    # live interview: the candidate sits in silence for the whole of it. The
    # bound is short enough that giving up and asking something else is faster
    # than waiting, and it is configuration because a self-hosted endpoint on
    # modest hardware is legitimately slower than a hosted one.
    timeout_seconds: float = DEFAULT_TIMEOUT_SECONDS

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
            # Kept as written so validate() can name what was wrong with it;
            # parsing here would turn "thirty" into the default and say
            # nothing.
            timeout_seconds=_parse_timeout(env.get("PREPEET_LLM_TIMEOUT_SECONDS", "")),
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
    if not config.timeout_seconds > 0:
        # NaN arrives here from an unparseable value and fails this comparison
        # too, which is the point of using it as the sentinel.
        raise ModelConfigError("PREPEET_LLM_TIMEOUT_SECONDS must be a positive number of seconds")
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
    # One question's budget, and how many consecutive provider failures are
    # absorbed before the interview ends rather than continuing on fallbacks.
    # Two, because one failure is a blip and a third would mean the candidate
    # is answering questions no model chose, which is worse than a clean end.
    timeout_seconds: float = DEFAULT_TIMEOUT_SECONDS
    max_consecutive_failures: int = 2
    _history: list[dict[str, str]] = field(default_factory=list)
    _asked: int = 0
    _consecutive_failures: int = 0

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
        """Ask the model, degrading to a fallback question when it cannot answer.

        A live interview has a person sitting in whatever silence a provider
        makes, so neither a hang nor an error may reach the conversation loop:
        this used to await the completion bare, and a rate limit on question
        four ended the session. The timeout is applied here rather than only on
        the client so the guarantee holds for any provider, including one whose
        SDK ignores the argument.
        """
        self._history.append({"role": "user", "content": user_text})
        started = time.monotonic()
        try:
            raw = await asyncio.wait_for(
                self.complete(system_prompt(self.brief), list(self._history)),
                timeout=self.timeout_seconds,
            )
        except asyncio.CancelledError:
            # Redundant today, and deliberately kept: CancelledError is a
            # BaseException, so the clause below already lets it past. It is
            # here to say that a worker shutting down is not a provider fault,
            # because the tempting widening to `except BaseException` would
            # turn a shutdown into a fallback question. A test fails on that
            # widening; this clause is what the test is describing.
            raise
        except Exception as error:
            self._consecutive_failures += 1
            logger.warning(
                "the interviewer's model did not answer; asking a fallback question",
                exc_info=error,
                extra={
                    "model_version": self.version,
                    "consecutive_failures": self._consecutive_failures,
                },
            )
            # The history keeps the candidate's turn but not a question the
            # model never asked, so a recovered provider sees a truthful
            # exchange rather than words attributed to it.
            return self._fallback()
        self._consecutive_failures = 0
        reply = raw.strip()
        latency_ms = int((time.monotonic() - started) * 1000)
        if reply.startswith(END_MARKER) or not reply:
            return None
        # A model that keeps talking past its one question gets cut to it:
        # the first paragraph is the question, the rest is not spoken.
        text = reply.split("\n\n", 1)[0].strip()
        self._history.append({"role": "assistant", "content": text})
        self._asked += 1
        return Turn(text=text, latency_ms=latency_ms, model_version=self.version)

    def _fallback(self) -> Turn | None:
        """A question to carry one failed turn, or None once the provider is gone.

        Ending is the honest outcome for a provider that is not coming back: a
        fallback loop would have the candidate answering questions drawn from a
        list of four, which reads as an interview and is not one. The fallback
        counts against the question cap for the same reason the model's own
        questions do, so a degraded interview is not also a longer one.
        """
        if self._consecutive_failures > self.max_consecutive_failures:
            return None
        text = FALLBACK_QUESTIONS[self._asked % len(FALLBACK_QUESTIONS)]
        self._history.append({"role": "assistant", "content": text})
        self._asked += 1
        # The provenance says the model did not ask this. Every turn records
        # which model asked it, so the one case where none did has to say so.
        return Turn(text=text, latency_ms=0, model_version=f"{self.version}/fallback")

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

    client = AsyncAnthropic(
        api_key=config.api_key,
        base_url=config.base_url or None,
        timeout=config.timeout_seconds,
        # The interviewer gives up on its own budget, so a retrying client
        # would spend that budget on attempts nobody is waiting for any more.
        max_retries=0,
    )

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
        timeout=config.timeout_seconds,
        max_retries=0,
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
