# internal — bounded contexts

## What this owns

One package per bounded context from the domain model, each with domain, application, repository, transport and composition layers.

## What this must never do

A context never imports another context's internal packages, and never reads or writes tables it does not own. Cross-context work goes through an application service or an event.

## The one directory here that is not a context

`isolation` is SEC-02's adversarial tenant-isolation suite. It holds no production code and never will: only tests, which attack one request path through the HTTP handler, the bounded context and the database at once. That is why it is allowed to import more than one context, and the exemption is declared in `architecture/architecture_test.go`, conditional on the package staying free of production code.
