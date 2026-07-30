#!/bin/bash
# s69 code-size driver. Generates 4 programs (defn vs defexpand x 1 vs 50
# call sites), compiles each, and reports emitted-Go line count + stripped
# binary size. Run from the spike dir:
#   CLJGO_SRC=<repo> WORK=<scratch> bash gen-size.sh
set -e
WORK=${WORK:?set WORK}
BIN=${BIN:-/tmp/cljgo-s69}
mkdir -p "$WORK/size"
gen() { # $1=kind $2=n
  local kind=$1 n=$2 f="$WORK/size/$kind-$n.clj"
  {
    if [ "$kind" = defn ]; then echo '(defn add! [a x] (swap! a conj x))'
    else echo '(defexpand add! [a x] (swap! a conj x))'; fi
    echo '(def todo (atom []))'
    echo '(defn -main []'
    for i in $(seq 1 "$n"); do echo "  (add! todo $i)"; done
    echo '  (println (count @todo)))'
    echo '(-main)'
  } > "$f"
}
for kind in defn defexpand; do
  for n in 1 50 500; do
    gen $kind $n
    ( cd "$WORK/size" && $BIN build -gen "g-$kind-$n" -o "b-$kind-$n" "$kind-$n.clj" >/dev/null 2>&1 )
    lines=$(wc -l < "$WORK/size/g-$kind-$n/main.go")
    size=$(stat -f%z "$WORK/size/b-$kind-$n")
    echo "$kind n=$n  main.go=${lines} lines  binary=${size} bytes"
  done
done
