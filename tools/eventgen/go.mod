// The durable event catalogue generator, as its own module.
//
// Separate from services/platform for the reason authzgen is: a build-time JSON
// Schema reader must not become a runtime dependency of the service. The
// service ships the generated registry and parses no schema at startup.
module github.com/Yelethe1st/prepeet/tools/eventgen

go 1.25
