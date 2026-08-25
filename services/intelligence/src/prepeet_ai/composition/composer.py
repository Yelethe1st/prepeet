"""Session bundle composition over pinned registry artifacts.

The caller resolves and pins artifacts from the registry - the registry is
Go's - and composition reads only what arrives pinned. The bundle document
this produces records every pin: type, reference, version, schema version and
digest, which is CAT-02's requirement that a bundle can say exactly what
produced it. The bundle's own digest covers those pins, so two compositions
from the same pins converge and any difference in inputs is a different
bundle identity.

Every pin's digest is verified against its body before composition: a body
that does not match its pin would make the bundle assert inputs it did not
read, which is the reproducibility lie this module exists to prevent.

What stays a floor: no persona rendering, no plan interpretation, no provider
call. Those arrive with the runtime and evaluation capabilities; the bundle's
record-keeping is complete now, which is what every later stage pins against.
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass

from prepeet_ai.transport.envelope import Failure, FailureCode, FailureError

SCHEMA_VERSION = "0.2"
"""The bundle schema. 0.2 records pinned inputs; 0.1 recorded identifiers
only and exists in no published bundle, the registry having arrived first."""

CALCULATION_VERSION = "compose-2"
"""Named so a bundle can say which composition logic produced it."""


@dataclass(frozen=True, slots=True)
class Pinned:
    """One registry artifact as the caller pinned it."""

    artifact_type: str
    reference: str
    version: str
    schema_version: str
    digest: str
    body: bytes


@dataclass(frozen=True, slots=True)
class ComposedBundle:
    """What composition hands back: the document, and the digest that is it."""

    storage_key: str
    digest: str
    revision: int
    body: bytes


def _digest_of(body: bytes) -> str:
    """The registry's canonical digest, recomputed here for verification.

    Canonicalisation must match Go's: JSON re-encoded with sorted keys and no
    whitespace. The cross-language test in cmd/worker is what holds the two
    implementations to each other.
    """
    decoded = json.loads(body)
    canonical = json.dumps(decoded, sort_keys=True, separators=(",", ":"))
    return "sha256:" + hashlib.sha256(canonical.encode()).hexdigest()


def compose(
    session_id: str,
    blueprint_id: str,
    purpose: str,
    pinned: list[Pinned],
) -> ComposedBundle:
    """Compose the bundle for one session from its pinned inputs.

    Raises:
        FailureError: INVALID_INPUT for a request missing what composition
            needs, or carrying a pin whose body does not match its digest.
            ARTIFACT_NOT_FOUND when the requested blueprint is not among the
            pinned plans.
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
    if not pinned:
        raise FailureError(
            Failure(
                code=FailureCode.INVALID_INPUT,
                message="composition reads only pinned inputs, and none arrived",
            )
        )

    for pin in pinned:
        try:
            recomputed = _digest_of(pin.body)
        except (ValueError, UnicodeDecodeError) as error:
            raise FailureError(
                Failure(
                    code=FailureCode.INVALID_INPUT,
                    message=f"the body pinned for {pin.reference} is not valid JSON",
                )
            ) from error
        if recomputed != pin.digest:
            raise FailureError(
                Failure(
                    code=FailureCode.INVALID_INPUT,
                    message=f"the body pinned for {pin.reference} does not match its digest",
                    detail={"reference": pin.reference},
                )
            )

    # The blueprint must be among the pins as a plan: composing against a plan
    # the request did not pin would give the bundle an input its record omits.
    if not any(pin.artifact_type == "plan" and pin.reference == blueprint_id for pin in pinned):
        raise FailureError(
            Failure(
                code=FailureCode.ARTIFACT_NOT_FOUND,
                message=f"the pinned inputs carry no plan for blueprint {blueprint_id}",
                detail={"blueprint_id": blueprint_id},
            )
        )

    document = {
        "schema_version": SCHEMA_VERSION,
        "calculation_version": CALCULATION_VERSION,
        "session_id": session_id,
        "purpose": purpose,
        "blueprint_id": blueprint_id,
        # Sorted for determinism: the same pins in any order are the same
        # bundle, because they are the same inputs.
        "pinned_inputs": sorted(
            (
                {
                    "artifact_type": pin.artifact_type,
                    "reference": pin.reference,
                    "version": pin.version,
                    "schema_version": pin.schema_version,
                    "digest": pin.digest,
                }
                for pin in pinned
            ),
            key=lambda entry: (str(entry["artifact_type"]), str(entry["reference"])),
        ),
    }

    body = json.dumps(document, sort_keys=True, separators=(",", ":")).encode()
    digest = "sha256:" + hashlib.sha256(body).hexdigest()

    return ComposedBundle(
        storage_key=f"sessions/{session_id}/bundle",
        digest=digest,
        revision=1,
        body=body,
    )
