# internal — bounded contexts

## What this owns

One package per bounded context from the domain model, each with domain, application, repository, transport and composition layers.

## What this must never do

A context never imports another context's internal packages, and never reads or writes tables it does not own. Cross-context work goes through an application service or an event.
