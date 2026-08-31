"""Continuing the caller's trace across the language boundary.

PLT-08 asks for one trace with no broken links, and this plane was the widest
break in it. The Go worker sends W3C trace context on every call; nothing here
read it, so extraction, evidence and articulation, which are the slowest work in
the product, could not be connected to the request that caused them.

The format is W3C trace context, the same one the outbox stores and the Go
client sends. That is the point of a standard: no part of this journey needs a
private agreement with another part in order to be joined up.
"""

from __future__ import annotations

import functools
from collections.abc import Iterator, Mapping
from contextlib import contextmanager

import grpc
from opentelemetry import context as otel_context
from opentelemetry.instrumentation.grpc import server_interceptor
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator

# Named rather than taken from the global propagator, which is a noop until
# something installs one. Depending on the global would mean a process that
# traced correctly and a test that silently propagated nothing, and the
# difference would surface only as a broken link in production. The Go side
# names its propagator for the same reason.
_propagator = TraceContextTextMapPropagator()


@contextmanager
def continued_context(carrier: Mapping[str, str]) -> Iterator[None]:
    """Run the block inside the trace the caller sent, if there is one.

    An absent traceparent yields a fresh trace rather than an error: work with
    no request behind it is a real state, and refusing to trace it would lose
    the very spans an operator needs when something runs on its own.

    A traceparent that does not parse is ignored for a sharper reason. The
    propagator answers a context with no span rather than raising, and attaching
    to it would produce spans pointing at a trace that cannot exist, which looks
    joined and leads nowhere. That is worse than an honest root.
    """
    extracted = _propagator.extract(carrier)
    token = otel_context.attach(extracted)
    try:
        yield
    finally:
        otel_context.detach(token)


@functools.lru_cache(maxsize=1)
def tracing_interceptor() -> grpc.ServerInterceptor:
    """The server interceptor that joins each call to its caller's trace.

    Cached because a second interceptor on the same server would produce two
    spans for every call, and a duplicated span is harder to notice than a
    missing one: the trace still looks complete.
    """
    # The instrumentation package ships no type information, so mypy sees an
    # untyped call returning Any. Narrowed here rather than by loosening the
    # strictness for the whole module, which would also stop it checking the
    # code around this line.
    interceptor: grpc.ServerInterceptor = server_interceptor()  # type: ignore[no-untyped-call]
    return interceptor
