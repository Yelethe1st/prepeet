# The Python intelligence plane.
#
# Pinned by digest for the reason services/platform/Dockerfile gives: a tag is a
# moving pointer, and an artifact whose inputs can change is not immutable.

FROM python:3.12-slim@sha256:09f7da3bc104798d0afb40bc08d23ab2da20a76130cec1f2ef170848f5d85217 AS build

# uv is the toolchain this service already uses, so the image installs the same
# way a contributor does. A second dependency mechanism here would be a second
# thing to keep in step with pyproject.toml.
# Pinned to the version that wrote uv.lock. The lockfile carries a revision
# number and an older uv simply cannot parse it, so "whatever is latest" is a
# build that breaks on a day nobody changed anything.
COPY --from=ghcr.io/astral-sh/uv:0.8.24@sha256:1d31be550ff927957472b2a491dc3de1ea9b5c2d319a9cea5b6a48021e2990a6 /uv /bin/uv

# The repository layout is mirrored rather than flattened. uv.lock names the
# generated contracts package as ../../packages/generated/python, relative to
# the service, and a flattened image makes that path resolve above the root.
# Copying the tree as it is on disk is simpler than rewriting the lockfile, and
# it keeps the image honest about where things actually live.
WORKDIR /app/services/intelligence

# The lockfile decides. --frozen refuses to resolve anything it does not already
# name, so an image cannot quietly acquire a version no contributor has.
COPY packages/generated/python /app/packages/generated/python
COPY services/intelligence/pyproject.toml services/intelligence/uv.lock ./
COPY services/intelligence/README.md ./
RUN uv sync --frozen --no-install-project --no-dev

COPY services/intelligence/src ./src
COPY services/intelligence/evals ./evals
RUN uv sync --frozen --no-dev

FROM python:3.12-slim@sha256:09f7da3bc104798d0afb40bc08d23ab2da20a76130cec1f2ef170848f5d85217

# A user with no privileges and no home to write into. The service reads its
# configuration from the environment and writes nothing to disk.
RUN useradd --system --no-create-home --uid 10001 prepeet

COPY --from=build /app /app
WORKDIR /app/services/intelligence

ENV PATH="/app/services/intelligence/.venv/bin:$PATH" \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

USER prepeet
ENTRYPOINT ["python", "-m", "prepeet_ai.transport.server"]
