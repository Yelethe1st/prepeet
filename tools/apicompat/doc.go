// apicompat refuses a change to the HTTP contract that would break a client
// already deployed against the previous release.
//
// CTR-04's second criterion. The event catalogue and the RPC contracts have
// had this gate since CTR-03; the OpenAPI document did not, so removing an
// endpoint, dropping a required response field or making a request field
// mandatory passed CI and broke a consumer at run time instead.
//
// Consumer-facing only, which is the same rule the event checker follows and
// for the same reason: a change that makes life harder for the server is
// caught by the compiler, and a change that breaks a client is caught here or
// in production.
//
// What it deliberately does not flag, so that a green result means something:
//
//	Adding a path, an operation, an optional request property or a response
//	property. All are additive for an existing client.
//
//	Adding a value to a response enum. Strictly this can break a client with
//	an exhaustive switch, and flagging it would fire on almost every release
//	while the product grows. It is a documented gap rather than a silent one.
//
//	Anything about descriptions, examples or ordering. They are not the wire.
package main
