#!/usr/bin/env bash
# Strips generated files from a Go coverage profile, and prints the total.
#
# Generated code is not what a coverage floor is for. sqlc emits one package
# per module (ADR-0010) and none of them can ever hold a test of their own:
# `go test -coverprofile` credits a package only for tests inside it, so every
# `*/db` package reads 0% however thoroughly the store above it is exercised.
# The effect compounds: each new sqlc module lowers the measured number while
# the real testing improves, which is a measurement that punishes the thing it
# exists to encourage.
#
# What keeps generated code honest is elsewhere and stronger: `make
# check-generated` fails if it does not match the contracts, and the
# integration suites run every query against real PostgreSQL.
#
# One implementation, used by the Makefile and by CI, because two copies of a
# check are two chances for one of them to stop being run.
#
# Usage: handwritten.sh <coverage-profile>
set -euo pipefail

profile="${1:?usage: handwritten.sh <coverage-profile>}"
filtered="$(mktemp)"
trap 'rm -f "$filtered"' EXIT

# The first line is the mode header, which `go tool cover` requires.
awk 'NR == 1 { print; next } $0 !~ /\.gen\.go/' "$profile" > "$filtered"
go tool cover -func="$filtered" | awk '/^total:/ { gsub(/%/, "", $3); print $3 }'
