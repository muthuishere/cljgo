#!/usr/bin/env bash
# S33 fixture: LOCAL BARE GIT REPOS as the remotes. No network, no github.
# Usage: fixture.sh <workdir>
set -euo pipefail
W="${1:?workdir}"
# published cache entries are chmod 0555 (the "you are editing the cache"
# tripwire), so a plain rm -rf cannot remove a previous run.
chmod -R u+w "$W" 2>/dev/null || true
rm -rf "$W"; mkdir -p "$W/remotes" "$W/src"

export GIT_AUTHOR_NAME=s28 GIT_AUTHOR_EMAIL=s28@example.invalid
export GIT_COMMITTER_NAME=s28 GIT_COMMITTER_EMAIL=s28@example.invalid
export GIT_AUTHOR_DATE="2026-01-01T00:00:00Z" GIT_COMMITTER_DATE="2026-01-01T00:00:00Z"

mkrepo() { # mkrepo <name> ; expects $W/src/<name> populated
  local n="$1"
  git -C "$W/src/$n" init -q -b main
  git -C "$W/src/$n" add -A
  git -C "$W/src/$n" commit -q -m "$n v1"
  git -C "$W/src/$n" tag v1.0.0
  git clone -q --bare "$W/src/$n" "$W/remotes/$n.git"
}

# --- acme-util: pure leaf --------------------------------------------------
mkdir -p "$W/src/acme-util/src/acme"
cat > "$W/src/acme-util/cljgo.manifest.edn" <<'EOF'
;; Declarative manifest (ADR 0048 decision 5): resolution reads THIS,
;; it never evaluates build.cljgo's (defn build [b] ...).
{:name "acme-util"
 :paths ["src"]
 :deps []}
EOF
cat > "$W/src/acme-util/src/acme/util.clj" <<'EOF'
(ns acme.util)
(defn greet [x] (str "hello " x))
EOF
cat > "$W/src/acme-util/build.cljgo" <<'EOF'
(defn build [b] (lib b {:name "acme-util"}))
EOF
mkrepo acme-util

# --- acme-http: depends on acme-util, IMPURE via go-require ----------------
mkdir -p "$W/src/acme-http/src/acme"
cat > "$W/src/acme-http/cljgo.manifest.edn" <<EOF
{:name "acme-http"
 :paths ["src"]
 :deps [{:name "acme-util" :git "file://$W/remotes/acme-util.git" :ref "v1.0.0"}]
 :go-require [{:module "github.com/gorilla/websocket" :version "v1.5.3"}]}
EOF
cat > "$W/src/acme-http/src/acme/http.clj" <<'EOF'
(ns acme.http (:require [acme.util :as u]))
(defn serve [] (u/greet "http"))
EOF
mkrepo acme-http

# --- acme-crypt: IMPURE via c-link (cgo) -----------------------------------
mkdir -p "$W/src/acme-crypt/src/acme"
cat > "$W/src/acme-crypt/cljgo.manifest.edn" <<'EOF'
{:name "acme-crypt"
 :paths ["src"]
 :deps []
 :c-link [{:pkg-config "libsodium"}]
 :ffi [{:lib "sodium"}]}
EOF
cat > "$W/src/acme-crypt/src/acme/crypt.clj" <<'EOF'
(ns acme.crypt)
(defn box [] :sealed)
EOF
mkrepo acme-crypt

# --- the consuming project -------------------------------------------------
mkdir -p "$W/proj/src" "$W/proj/local-lib/src/local"
cat > "$W/proj/build.cljgo" <<EOF
(defn build [b]
  (dep b "acme-http"  {:git "file://$W/remotes/acme-http.git"  :ref "v1.0.0"})
  (dep b "acme-crypt" {:git "file://$W/remotes/acme-crypt.git" :ref "v1.0.0"})
  (dep b "local-lib"  {:path "local-lib"})
  (exe b {:name "app" :main "app.core"}))
EOF
cat > "$W/proj/local-lib/cljgo.manifest.edn" <<'EOF'
{:name "local-lib" :paths ["src"] :deps []}
EOF
cat > "$W/proj/local-lib/src/local/lib.clj" <<'EOF'
(ns local.lib)
(defn x [] 1)
EOF
echo "fixture ready: $W"
