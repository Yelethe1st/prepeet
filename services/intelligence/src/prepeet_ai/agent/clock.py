"""The room clock: milliseconds since the agent joined.

ADR-0013 makes the SFU's room timebase the only clock. The agent has no
access to the SFU's internal clock, so it anchors at join and measures from
there; every event it writes is on this one monotonic scale, which is what
alignment against the recording's start offset needs.
"""

from __future__ import annotations

import time


class RoomClock:
    """Monotonic milliseconds from an anchor."""

    def __init__(self, now: float | None = None) -> None:
        """Anchor at the given monotonic instant, or at construction."""
        self._anchor = time.monotonic() if now is None else now

    def now_ms(self, now: float | None = None) -> int:
        """Milliseconds elapsed since the anchor."""
        instant = time.monotonic() if now is None else now
        return max(0, int((instant - self._anchor) * 1000))
