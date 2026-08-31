"""One trace across the language boundary.

PLT-08's open criterion is a single trace with no broken links. This plane was
the widest break in it: the Go worker sends W3C trace context on every call, and
nothing here read it, so extraction, evidence and articulation, which are the
slowest work in the product, could not be connected to the request that caused
them.

These assert continuity rather than the presence of a library. The question a
person asks of a trace is "what happened to that request", and the answer is
worthless if the id changes at the boundary.
"""

from __future__ import annotations

import grpc
import pytest
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from opentelemetry.trace import format_trace_id
from prepeet.intelligence.v1 import intelligence_pb2, intelligence_pb2_grpc

from prepeet_ai.transport import server as transport
from prepeet_ai.transport.tracing import continued_context, tracing_interceptor

# A traceparent as the Go side sends it: version, trace id, span id, flags.
CALLER_TRACE = "4bf92f3577b34da6a3ce929d0e0e4736"
CALLER_PARENT = f"00-{CALLER_TRACE}-00f067aa0ba902b7-01"


@pytest.fixture(name="recorder")
def _recorder():
    exporter = InMemorySpanExporter()
    provider = TracerProvider()
    provider.add_span_processor(SimpleSpanProcessor(exporter))
    return provider, exporter


class TestContinuity:
    """Extraction: whether the caller's trace survives the boundary at all."""

    def test_a_call_carrying_a_traceparent_continues_that_trace(self, recorder) -> None:
        """The whole point: one trace id on both sides of the boundary."""
        provider, exporter = recorder
        with continued_context({"traceparent": CALLER_PARENT}):
            provider.get_tracer("test").start_span("compose").end()

        spans = exporter.get_finished_spans()
        assert len(spans) == 1
        assert format_trace_id(spans[0].context.trace_id) == CALLER_TRACE

    def test_a_call_with_no_traceparent_starts_its_own_trace(self, recorder) -> None:
        """A worker outside a trace is a real state, not an error."""
        provider, exporter = recorder
        with continued_context({}):
            provider.get_tracer("test").start_span("compose").end()

        spans = exporter.get_finished_spans()
        assert len(spans) == 1
        assert spans[0].parent is None
        assert spans[0].context.trace_id != 0

    def test_a_malformed_traceparent_is_ignored_rather_than_trusted(self, recorder) -> None:
        """Metadata is data, and data arrives wrong.

        A span pointing at a trace that cannot exist is worse than a span with
        no parent, because it looks joined and leads nowhere.
        """
        provider, exporter = recorder
        with continued_context({"traceparent": "not-a-traceparent"}):
            provider.get_tracer("test").start_span("compose").end()

        assert exporter.get_finished_spans()[0].parent is None

    def test_the_caller_is_the_parent_not_merely_the_same_trace(self, recorder) -> None:
        """Same trace is not enough; the span must hang under the caller's."""
        provider, exporter = recorder
        with continued_context({"traceparent": CALLER_PARENT}):
            provider.get_tracer("test").start_span("compose").end()

        parent = exporter.get_finished_spans()[0].parent
        assert parent is not None
        assert f"{parent.span_id:016x}" == "00f067aa0ba902b7"


class TestServing:
    """Reachability: whether that extractor is actually on the serving path."""

    def test_a_real_call_produces_a_server_span_in_the_callers_trace(self) -> None:
        """The mechanism above proves extraction; this proves it is reached.

        A correct extractor nobody calls is the same broken link with more code
        behind it, and an earlier version of this test only asserted the call
        raised, which it does whether or not the interceptor exists. Removing
        the interceptor left that test green, so it was replaced with one that
        reads the spans.

        The instrumentation resolves its tracer from the global provider, so the
        provider is installed for the process before the server is built. Set
        once, guarded, because OpenTelemetry refuses a second installation and
        would otherwise make this test order-dependent.
        """
        exporter = _install_global_provider()
        exporter.clear()

        served, port = transport.serve(port=0)
        try:
            stub = intelligence_pb2_grpc.IntelligenceServiceStub(
                grpc.insecure_channel(f"localhost:{port}")
            )
            with pytest.raises(grpc.RpcError):
                # Refused for having no session, which is the point: a refusal
                # is an answer, and the span is recorded either way.
                stub.ComposeSessionBundle(
                    intelligence_pb2.ComposeSessionBundleRequest(),
                    metadata=(("traceparent", CALLER_PARENT),),
                    timeout=10,
                )
        finally:
            served.stop(0)

        traces = {format_trace_id(span.context.trace_id) for span in exporter.get_finished_spans()}
        assert traces, "the server recorded no span at all, so nothing joins the caller's trace"
        assert CALLER_TRACE in traces, (
            f"the server span is in {traces} rather than the caller's trace {CALLER_TRACE}: "
            "the trace breaks at the language boundary"
        )

    def test_the_interceptor_is_built_once_rather_than_per_call(self) -> None:
        """Two interceptors would double every span on the serving path."""
        assert tracing_interceptor() is tracing_interceptor()


_GLOBAL_EXPORTER: InMemorySpanExporter | None = None


def _install_global_provider() -> InMemorySpanExporter:
    """Install one recording provider for the process, once."""
    global _GLOBAL_EXPORTER
    if _GLOBAL_EXPORTER is None:
        _GLOBAL_EXPORTER = InMemorySpanExporter()
        provider = TracerProvider()
        provider.add_span_processor(SimpleSpanProcessor(_GLOBAL_EXPORTER))
        trace.set_tracer_provider(provider)
    return _GLOBAL_EXPORTER
