"""Session bundle composition: the walking-skeleton capability.

This is CAT-02's floor, not its ceiling, and it says so. Real composition
reads pinned artifacts - persona, plan, rules, rubric - renders them into the
immutable bundle, stores it, and returns the reference. None of those
artifacts exist yet. What exists is the property every later stage depends on
and the walking skeleton must prove end to end: composition is deterministic
over its pinned inputs, so the same request produces the same digest, and a
retried activity converges instead of forking.

The digest is computed over a canonical encoding of the inputs. When real
artifacts arrive, they join the encoding and the schema version moves; the
contract's response meta is what tells a caller which composition produced
what it holds.
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass

from prepeet_ai.transport.envelope import Failure, FailureCode, FailureError

SCHEMA_VERSION = "0.1"
"""The bundle schema this composer produces. Pre-1.0 on purpose: the bundle
holds only its pinned identifiers until the artifact registry exists."""

CALCULATION_VERSION = "compose-skeleton-1"
"""Named so a bundle can say which composition logic produced it, which is the
reproducibility contract even for a skeleton."""


@dataclass(frozen=True, slots=True)
class ComposedBundle:
    """What composition hands back: a reference and the digest that pins it."""

    storage_key: str
    digest: str
    revision: int


def compose(session_id: str, blueprint_id: str, purpose: str) -> ComposedBundle:
    """Compose the bundle for one session.

    Deterministic by construction: the digest covers exactly the pinned
    inputs, in canonical form, so a retry after a worker death converges on
    the same bundle rather than forking the session's identity.

    Raises:
        FailureError: INVALID_INPUT for a request missing what composition needs.
            Non-retryable by the contract's own declaration - the same
            request will fail the same way.
    """
    if not session_id:
        raise FailureError(
            Failure(code=FailureCode.INVALID_INPUT, message="a session id is required")
        )
    if not blueprint_id:
        raise FailureError(
            Failure(code=FailureCode.INVALID_INPUT, message="a blueprint id is required")
        )
    if purpose not in ("practice", "screening"):
        raise FailureError(
            Failure(
                code=FailureCode.INVALID_INPUT,
                message="the purpose must be practice or screening",
            )
        )

    canonical = json.dumps(
        {
            "schema_version": SCHEMA_VERSION,
            "session_id": session_id,
            "blueprint_id": blueprint_id,
            "purpose": purpose,
        },
        sort_keys=True,
        separators=(",", ":"),
    )
    digest = hashlib.sha256(canonical.encode()).hexdigest()

    return ComposedBundle(
        storage_key=f"bundles/{session_id}",
        digest=f"sha256:{digest}",
        revision=1,
    )
