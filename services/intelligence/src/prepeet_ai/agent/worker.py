"""The LiveKit worker: joins rooms as the interviewer and runs a conversation.

Everything here is SDK and provider glue, constructed from deployment
configuration and exercised against a live room, which is why it is
excluded from unit coverage: the orchestration it hands to is proven in
conversation.py against fakes.

Run with: uv run python -m prepeet_ai.agent.worker dev
"""

from __future__ import annotations

import logging
import os
from typing import Any

logger = logging.getLogger(__name__)


def main() -> None:  # pragma: no cover - LiveKit process entrypoint
    """Start the agent worker against the configured SFU."""
    from livekit import agents, rtc

    from prepeet_ai.agent.clock import RoomClock
    from prepeet_ai.agent.conversation import Conversation
    from prepeet_ai.agent.model import ModelConfig, ModelInterviewer, completer_from_config
    from prepeet_ai.agent.ports import Interviewer
    from prepeet_ai.agent.providers import NoSpeech, ProviderConfig
    from prepeet_ai.agent.scripted import ScriptedInterviewer
    from prepeet_ai.agent.timeline import PlatformTimeline, TimelineTarget, fetch_brief

    async def entrypoint(ctx: agents.JobContext) -> None:
        await ctx.connect(auto_subscribe=agents.AutoSubscribe.AUDIO_ONLY)
        clock = RoomClock()
        room: rtc.Room = ctx.room

        # The candidate joined as their user id (SES-02's grant); the room
        # name is the session id. Both are what the timeline needs.
        candidate_id = next(
            (p.identity for p in room.remote_participants.values() if p.identity != "interviewer"),
            "",
        )
        if not candidate_id:
            logger.warning("no candidate in room %s yet; waiting", room.name)
            participant = await ctx.wait_for_participant()
            candidate_id = participant.identity

        timeline_target = TimelineTarget(
            api_url=os.environ.get("PREPEET_API_URL", "http://localhost:8080"),
            service_token=os.environ.get("PREPEET_AGENT_TOKEN", ""),
            session_id=room.name,
            candidate_id=candidate_id,
        )
        timeline = PlatformTimeline(timeline_target)

        providers = ProviderConfig.from_env()
        if not providers.complete:
            logger.error(
                "providers not configured (PREPEET_DEEPGRAM_API_KEY, PREPEET_CARTESIA_API_KEY); "
                "the interviewer will speak nothing and hear nothing"
            )
            stt = NoSpeech()
            tts = _SilentSpeaker()
        else:
            stt, tts = _live_providers(ctx, room, clock, providers)

        interviewer: Interviewer = ScriptedInterviewer(
            "Welcome to your practice interview. When you are ready, tell me about a piece "
            "of work you led recently.",
            [
                "What was the hardest decision in that work, and what did you weigh?",
                "What would you do differently next time?",
            ],
        )
        model_config = ModelConfig.from_env(os.environ)
        if model_config is not None:
            try:
                brief = await fetch_brief(timeline_target)
                interviewer = ModelInterviewer(
                    brief=brief,
                    complete=completer_from_config(model_config),
                    version=model_config.version,
                )
            except Exception:  # the scripted floor is the fallback
                logger.exception(
                    "model interviewer unavailable; falling back to the scripted interviewer"
                )

        conversation = Conversation(
            interviewer=interviewer,
            stt=stt,
            tts=tts,
            timeline=timeline,
            clock=clock,
        )
        await conversation.run()

    agents.cli.run_app(agents.WorkerOptions(entrypoint_fnc=entrypoint, agent_name="interviewer"))


class _SilentSpeaker:  # pragma: no cover - misconfiguration fallback
    async def speak(self, text: str) -> int:
        logger.info("would say: %s", text)
        return 100 * len(text.split())


def _live_providers(  # pragma: no cover
    ctx: Any, room: Any, clock: Any, providers: Any
) -> tuple[Any, Any]:
    """Deepgram and Cartesia through LiveKit's plugins, mapped onto our ports."""
    from livekit import rtc
    from livekit.plugins import cartesia, deepgram

    from prepeet_ai.agent.providers import segment_from_deepgram

    stt_plugin = deepgram.STT(api_key=providers.deepgram_api_key, model="nova-3")
    tts_plugin = (
        cartesia.TTS(api_key=providers.cartesia_api_key, voice=providers.cartesia_voice)
        if providers.cartesia_voice
        else cartesia.TTS(api_key=providers.cartesia_api_key)
    )

    class Hearing:
        async def segments(self):  # type: ignore[no-untyped-def]
            participant = await ctx.wait_for_participant()
            track = None
            for publication in participant.track_publications.values():
                if publication.track and publication.kind == rtc.TrackKind.KIND_AUDIO:
                    track = publication.track
            if track is None:
                return
            offset_ms = clock.now_ms()
            stream = stt_plugin.stream()
            audio = rtc.AudioStream(track)

            async def pump() -> None:
                async for frame in audio:
                    stream.push_frame(frame.frame)
                stream.end_input()

            import asyncio

            pumping = asyncio.create_task(pump())
            try:
                async for event in stream:
                    if event.type != agents_stt_final():
                        continue
                    alternative = event.alternatives[0]
                    words = [
                        (
                            w.word,
                            w.start_time,
                            w.end_time,
                            getattr(w, "confidence", alternative.confidence),
                        )
                        for w in getattr(alternative, "words", [])
                    ]
                    yield segment_from_deepgram(
                        alternative.text, words, offset_ms, alternative.confidence
                    )
            finally:
                pumping.cancel()

    class Speaking:
        def __init__(self) -> None:
            self.source = rtc.AudioSource(sample_rate=24000, num_channels=1)
            self.track = rtc.LocalAudioTrack.create_audio_track("interviewer", self.source)
            self.published = False

        async def speak(self, text: str) -> int:
            if not self.published:
                await room.local_participant.publish_track(self.track)
                self.published = True
            started = clock.now_ms()
            async for chunk in tts_plugin.synthesize(text):
                await self.source.capture_frame(chunk.frame)
            return int(max(1, clock.now_ms() - started))

    return Hearing(), Speaking()


def agents_stt_final() -> Any:  # pragma: no cover
    """The plugin's final-transcript event type, imported lazily."""
    from livekit.agents import stt

    return stt.SpeechEventType.FINAL_TRANSCRIPT


if __name__ == "__main__":  # pragma: no cover
    main()
