"""Transport layer: the typed boundary between Go and this service.

Nothing in this package knows what a competency or a session is. It carries
results and failures, and it insists both are versioned and typed.
"""

__all__ = ["envelope"]

from prepeet_ai.transport import envelope
