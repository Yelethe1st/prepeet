# The Go control plane: one image, two entrypoints.
#
# PLT-02's second criterion is that build artifacts are immutable and
# digest-addressed. Immutability starts at the inputs: every base image is
# pinned by digest rather than by tag, because a tag is a moving pointer and
# `golang:1.26-alpine` means something different next week. A build that cannot
# say exactly what it was built from cannot be reproduced, and an artifact
# nobody can reproduce is not immutable in any useful sense.
#
# api and worker share this file because they share a module and differ only in
# which main is compiled. Two files would be two places for a base image or a
# build flag to drift apart.

FROM golang:1.26-alpine@sha256:28d89ee9cc0ff9fec75c82ca201e6bf7fdf9a679d4b7b24dfa04f2bb766bb468 AS build

WORKDIR /src

# Dependencies before source, so a change to the code does not re-download the
# module cache. The two files are copied alone for the same reason.
COPY services/platform/go.mod services/platform/go.sum ./services/platform/
COPY packages/generated/go ./packages/generated/go
RUN cd services/platform && go mod download

COPY services/platform ./services/platform
COPY packages ./packages

# Which command to build. Passed in rather than duplicated across files.
ARG COMMAND=api

# CGO off so the binary is static and can run on a base with no libc at all.
# trimpath and the empty buildid strip the build machine's paths and a
# timestamp-derived id from the output, which is what makes two builds of the
# same commit produce the same bytes. Without it the digest changes for reasons
# that have nothing to do with the code.
RUN cd services/platform && CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w -buildid=" -o /out/service ./cmd/${COMMAND}

# Distroless: no shell, no package manager, nothing to exec into. A static Go
# binary needs none of it, and every binary that is not there is one that cannot
# be used against us. nonroot because nothing here needs to be root, and the
# image says so rather than relying on the platform to impose it.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

COPY --from=build /out/service /service

USER nonroot:nonroot
ENTRYPOINT ["/service"]
