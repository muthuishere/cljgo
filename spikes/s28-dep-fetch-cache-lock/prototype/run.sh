#!/usr/bin/env bash
# S28 experiment runner. Writes every captured transcript into results/.
# Usage: run.sh <workdir>
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SPIKE="$(dirname "$HERE")"
R="$SPIKE/results"
W="${1:?workdir}"
BIN="$SPIKE/s28.bin"
mkdir -p "$R"

hdr() { echo; echo "### $*"; }

bash "$HERE/fixture.sh" "$W" >/dev/null
( cd "$HERE" && go build -o "$BIN" . ) || exit 1

##############################################################################
{
hdr "E1a  machine A: cold cache, no lock -> resolve -update"
CLJGO_CACHE="$W/cacheA" "$BIN" resolve -project "$W/proj" -update 2>&1

hdr "E1b  machine B: DIFFERENT project dir, DIFFERENT cache root, cold"
rm -rf "$W/projB"; mkdir -p "$W/projB"
cp -R "$W/proj/build.cljgo" "$W/proj/local-lib" "$W/projB/"
CLJGO_CACHE="$W/cacheB" "$BIN" resolve -project "$W/projB" -update 2>&1

hdr "E1c  the two lockfiles, byte-compared"
cmp "$W/proj/build.lock.edn" "$W/projB/build.lock.edn" && echo "cmp: IDENTICAL (exit $?)"

hdr "E1d  the two materialized trees, content-hash compared"
for d in $(CLJGO_CACHE="$W/cacheA" "$BIN" loadpath -project "$W/proj" 2>/dev/null); do
  case "$d" in "$W"/cache*) echo "A $("$BIN" treehash "$d") ${d##*/src/}";; esac
done | sort > "$W/hashA.txt"
for d in $(CLJGO_CACHE="$W/cacheB" "$BIN" loadpath -project "$W/projB" 2>/dev/null); do
  case "$d" in "$W"/cache*) echo "B $("$BIN" treehash "$d") ${d##*/src/}";; esac
done | sort > "$W/hashB.txt"
paste <(cut -d' ' -f2- "$W/hashA.txt") <(cut -d' ' -f2- "$W/hashB.txt")
diff <(cut -d' ' -f2- "$W/hashA.txt") <(cut -d' ' -f2- "$W/hashB.txt") && echo "diff: IDENTICAL DEPENDENCY TREES"
} > "$R/e1-determinism.txt" 2>&1

##############################################################################
{
hdr "E2a  warm-cache resolve from the lock (baseline: must pass)"
CLJGO_CACHE="$W/cacheA" "$BIN" resolve -project "$W/proj" 2>&1

hdr "E2b  tamper: append to one file inside a published cache entry"
V=$(CLJGO_CACHE="$W/cacheA" "$BIN" entry -project "$W/proj" -name acme-util 2>/dev/null)
echo "victim entry: ${V/$W/\$W}"
echo "perms before tamper: $(stat -f '%Sp' "$V/src/acme/util.clj")"
chmod -R u+w "$V"
echo ";; injected" >> "$V/src/acme/util.clj"
echo "appended 13 bytes to \$V/src/acme/util.clj"

hdr "E2c  next resolve"
CLJGO_CACHE="$W/cacheA" "$BIN" resolve -project "$W/proj" 2>&1 | sed "s|$W|\$W|g"
echo "exit=${PIPESTATUS[0]}"

hdr "E2d  repair (drop the corrupt entry) and re-resolve"
chmod -R u+w "$V"; rm -rf "$V"
CLJGO_CACHE="$W/cacheA" "$BIN" resolve -project "$W/proj" 2>&1 | sed "s|$W|\$W|g"
echo "exit=${PIPESTATUS[0]}"
} > "$R/e2-tamper.txt" 2>&1

##############################################################################
{
export GIT_AUTHOR_NAME=s28 GIT_AUTHOR_EMAIL=s28@example.invalid
export GIT_COMMITTER_NAME=s28 GIT_COMMITTER_EMAIL=s28@example.invalid
hdr "E3a  locked sha for acme-util, before"
grep -A1 'acme-util' "$W/proj/build.lock.edn" >/dev/null
awk '/acme-util.git/{print prev} {prev=$0}' "$W/proj/build.lock.edn"

hdr "E3b  ATTACK: force-move tag v1.0.0 in the remote to a new commit"
git -C "$W/src/acme-util" commit -q --allow-empty -m "malicious v1.0.0"
git -C "$W/src/acme-util" tag -f v1.0.0 >/dev/null 2>&1
git -C "$W/remotes/acme-util.git" fetch -q --force "$W/src/acme-util" '+refs/*:refs/*' 2>&1
echo "remote v1.0.0 now: $(git -C "$W/remotes/acme-util.git" rev-parse v1.0.0)"

hdr "E3c  LOCKED resolve (warm cache) — must be unaffected"
CLJGO_CACHE="$W/cacheA" "$BIN" resolve -project "$W/proj" 2>&1
echo "exit=$?"

hdr "E3d  LOCKED resolve on a COLD cache — must still fetch the OLD sha"
CLJGO_CACHE="$W/cacheD" "$BIN" resolve -project "$W/proj" 2>&1
echo "exit=$?"

hdr "E3e  explicit -update — the move is REPORTED, not silent"
cp "$W/proj/build.lock.edn" "$W/lock.before"
CLJGO_CACHE="$W/cacheA" "$BIN" resolve -project "$W/proj" -update 2>&1
echo "--- lock diff:"
diff "$W/lock.before" "$W/proj/build.lock.edn"
cp "$W/lock.before" "$W/proj/build.lock.edn"

hdr "E3g  second attack: force-move the tag onto a commit that CHANGES CONTENT"
echo '(ns acme.util) (defn greet [x] (str "pwned " x))' > "$W/src/acme-util/src/acme/util.clj"
git -C "$W/src/acme-util" commit -q -am "malicious content under v1.0.0"
git -C "$W/src/acme-util" tag -f v1.0.0 >/dev/null 2>&1
git -C "$W/remotes/acme-util.git" fetch -q --force "$W/src/acme-util" '+refs/*:refs/*' 2>&1
echo "-- locked resolve is STILL unaffected (identity is the sha, not the tag):"
CLJGO_CACHE="$W/cacheA" "$BIN" resolve -project "$W/proj" 2>&1 | tail -3 | sed "s|$W|\$W|g"
echo "-- and under -update BOTH fields move, so the diff is reviewable:"
cp "$W/proj/build.lock.edn" "$W/lock.before2"
CLJGO_CACHE="$W/cacheA" "$BIN" resolve -project "$W/proj" -update >/dev/null 2>&1
diff "$W/lock.before2" "$W/proj/build.lock.edn"
cp "$W/lock.before" "$W/proj/build.lock.edn"

hdr "E3f  build.cljgo asks for a ref the lock does not pin -> legible divergence"
sed -i.bak 's/acme-crypt.git" :ref "v1.0.0"/acme-crypt.git" :ref "v2.0.0"/' "$W/proj/build.cljgo"
CLJGO_CACHE="$W/cacheA" "$BIN" resolve -project "$W/proj" 2>&1
echo "exit=$?"
mv "$W/proj/build.cljgo.bak" "$W/proj/build.cljgo"
} > "$R/e3-moved-ref.txt" 2>&1

##############################################################################
{
hdr "E4a  OFFLINE: remotes physically renamed away, warm cache + lock"
mv "$W/remotes" "$W/remotes-GONE"
ls "$W" | sed 's/^/  /'
CLJGO_CACHE="$W/cacheA" "$BIN" resolve -project "$W/proj" -offline 2>&1
echo "exit=$?"

hdr "E4b  same, WITHOUT -offline (proves no remote is contacted when locked+warm)"
CLJGO_CACHE="$W/cacheA" "$BIN" resolve -project "$W/proj" 2>&1
echo "exit=$?"

hdr "E4c  offline + COLD cache -> must fail, legibly"
CLJGO_CACHE="$W/cacheZ" "$BIN" resolve -project "$W/proj" -offline 2>&1
echo "exit=$?"

hdr "E4d  VENDOR: populate vendor/ from the warm cache, then empty cache, offline"
chmod -R u+w "$W/proj/vendor" 2>/dev/null; rm -rf "$W/proj/vendor"; mkdir -p "$W/proj/vendor"
# what `cljgo vendor` would do: copy each resolved cache entry to vendor/<name>
for name in acme-http acme-crypt acme-util; do
  src=$(CLJGO_CACHE="$W/cacheA" "$BIN" entry -project "$W/proj" -name "$name" 2>/dev/null)
  cp -R "$src" "$W/proj/vendor/$name" && echo "vendored $name"
done
rm -rf "$W/cacheV"
echo "-- resolve with an EMPTY cache, offline, vendor/ present:"
CLJGO_CACHE="$W/cacheV" "$BIN" resolve -project "$W/proj" -offline 2>&1 | sed "s|$W|\$W|g"
echo "exit=${PIPESTATUS[0]}"
echo "-- and the load path now points into vendor/, not the cache:"
CLJGO_CACHE="$W/cacheV" "$BIN" loadpath -project "$W/proj" 2>/dev/null | sed "s|$W|\$W|g"

hdr "E4e  tamper the VENDOR copy -> caught by the same lock hash"
chmod -R u+w "$W/proj/vendor/acme-util"
echo ";; injected" >> "$W/proj/vendor/acme-util/src/acme/util.clj"
CLJGO_CACHE="$W/cacheV" "$BIN" resolve -project "$W/proj" -offline 2>&1 | sed "s|$W|\$W|g"
echo "exit=${PIPESTATUS[0]}"
chmod -R u+w "$W/proj/vendor"; rm -rf "$W/proj/vendor"
mv "$W/remotes-GONE" "$W/remotes"
} > "$R/e4-offline-vendor.txt" 2>&1

##############################################################################
{
hdr "E5  CONCURRENCY: 8 resolvers, one shared COLD cache"
rm -rf "$W/cacheC"
for i in $(seq 1 8); do
  ( CLJGO_CACHE="$W/cacheC" "$BIN" resolve -project "$W/proj" -quiet > "$W/c$i.out" 2>&1; echo "$?" > "$W/c$i.rc" ) &
done
wait
echo "exit codes: $(cat "$W"/c*.rc | tr '\n' ' ')"
echo "distinct outputs:"; sort -u "$W"/c*.out | sed 's/^/  /'
echo "cache src entries: $(ls "$W/cacheC/src" | grep -vc '^\.tmp' )"
echo "leftover temp dirs: $(ls -a "$W/cacheC/src" | grep -c '^\.tmp' )"
echo "verification pass after the storm:"
CLJGO_CACHE="$W/cacheC" "$BIN" resolve -project "$W/proj" 2>&1
} > "$R/e5-concurrency.txt" 2>&1

##############################################################################
{
hdr "E6  load path handed to S25's resolver (ADR 0048 §2 slot 3), lock order"
CLJGO_CACHE="$W/cacheA" "$BIN" loadpath -project "$W/proj" 2>/dev/null | sed "s|$W|\$W|"
hdr "E7  the lockfile"
sed "s|$W|\$W|g" "$W/proj/build.lock.edn"
hdr "E8  cache layout"
( cd "$W/cacheA" && find . -maxdepth 2 | sort | sed 's/^/  /' )
} > "$R/e6-loadpath-lock.txt" 2>&1

echo "done -> $R"
