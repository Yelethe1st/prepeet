"""The voice agent: the interviewer in the room, per ADR-0012 and ADR-0019.

The conversation logic is pure Python over ports (speech to text, text to
speech, the interviewer, the platform timeline), proven against fakes;
the LiveKit and provider glue lives in worker.py and providers.py and is
constructed only from deployment configuration.
"""
