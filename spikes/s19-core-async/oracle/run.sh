#!/usr/bin/env bash
# Oracle harness: real JVM Clojure 1.12.5 + core.async 1.6.681.
set -euo pipefail
cd "$(dirname "$0")"
clojure -Sdeps '{:deps {org.clojure/core.async {:mvn/version "1.6.681"}}}' \
        -M probes.clj
