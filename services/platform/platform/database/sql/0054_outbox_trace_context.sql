-- 0054: carry the trace across the outbox.
--
-- PLT-08. A request that publishes an event and a worker that later delivers
-- it are the same piece of work to everyone except the tracing system, which
-- until now saw two: the trace ended at the HTTP response and a second,
-- unrelated one began when the dispatcher picked the row up. Every question
-- worth asking of a trace spans that boundary, because the slow part of
-- creating an interview is rarely the request.
--
-- W3C traceparent and tracestate, stored as they travel. Not our own encoding:
-- the point of the standard is that the Python plane and any provider we hand
-- the context to can read it without agreeing with us first.
--
-- This is safe to keep in a table that reaches integrations and analytics.
-- A traceparent is a pair of random identifiers and a flag byte. It names no
-- person, carries no content, and is meaningless without the spans it points
-- at, which is the same reason it is safe to put on the wire.
--
-- Nullable rather than defaulted, because "published outside a trace" is a real
-- state and must stay distinguishable from "published inside one". Every row
-- written before this column existed is genuinely the former.

ALTER TABLE integration.outbox
    ADD COLUMN trace_parent text,
    ADD COLUMN trace_state  text;

COMMENT ON COLUMN integration.outbox.trace_parent IS
    'W3C traceparent of the span that published this event, so the worker that '
    'delivers it continues one trace rather than starting a second. Null when '
    'the publisher was not inside a trace, which is a real state and not a '
    'missing value.';

COMMENT ON COLUMN integration.outbox.trace_state IS
    'W3C tracestate accompanying trace_parent. Vendor-specific and often '
    'empty; carried rather than dropped so a downstream system that set it '
    'still sees it.';
