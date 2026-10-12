#!/bin/sh
set -e

# Bind-mounted data roots must exist before Denova binds the workspace.
mkdir -p "${DENOVA_DIR:-/data/.denova}" "${DENOVA_WORKSPACE:-/data/workspace}"

exec /app/denova "$@"
