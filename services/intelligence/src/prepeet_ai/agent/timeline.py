"""The platform timeline client: the agent writing the transcript it heard.

Speaks the internal ingest surface (ADR-0019): a service bearer token, the
session and the candidate identity from the room, and events without
sequences, because the server assigns them. Built on the standard library
so the agent's tests need no network stack; the request is small and
infrequent enough that a blocking call in a thread is honest.
"""

from __future__ import annotations

import asyncio
import json
import urllib.error
import urllib.request
from dataclasses import dataclass


class TimelineRefusedError(Exception):
    """The platform refused the batch: status and body for the log."""

    def __init__(self, status: int, body: str) -> None:
        """Record what the platform answered."""
        super().__init__(f"timeline answered {status}: {body}")
        self.status = status
        self.body = body


@dataclass(frozen=True)
class TimelineTarget:
    """Where and as whom the agent writes."""

    api_url: str
    service_token: str
    session_id: str
    candidate_id: str
    mode: str = "practice"


class PlatformTimeline:
    """POSTs event batches to the internal ingest surface."""

    def __init__(self, target: TimelineTarget) -> None:
        """Bind to one session's target."""
        self._target = target

    @property
    def url(self) -> str:
        """The internal ingest URL for this session."""
        base = self._target.api_url.rstrip("/")
        return f"{base}/api/v1/internal/interviews/{self._target.session_id}/events"

    async def post(self, events: list[dict[str, object]]) -> None:
        """Land one batch. Raises TimelineRefusedError on any non-200 answer."""
        await asyncio.to_thread(self._post_blocking, events)

    def _post_blocking(self, events: list[dict[str, object]]) -> None:
        body = json.dumps(
            {
                "candidate_id": self._target.candidate_id,
                "mode": self._target.mode,
                "events": events,
            }
        ).encode()
        request = urllib.request.Request(
            self.url,
            data=body,
            method="POST",
            headers={
                "Content-Type": "application/json",
                "Authorization": f"Bearer {self._target.service_token}",
            },
        )
        try:
            with urllib.request.urlopen(request, timeout=10) as response:
                response.read()
        except urllib.error.HTTPError as error:
            try:
                detail = error.read().decode(errors="replace")
            except OSError:
                detail = ""
            raise TimelineRefusedError(error.code, detail) from error
