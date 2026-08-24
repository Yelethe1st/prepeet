// The capability catalogue generator, as its own module.
//
// Separate from services/platform so that a build-time dependency on a YAML
// parser does not become a runtime dependency of the service. The service ships
// the generated file and parses nothing.
module github.com/Yelethe1st/prepeet/tools/authzgen

go 1.25

require gopkg.in/yaml.v3 v3.0.1
