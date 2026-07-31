#!/bin/bash
# s69 code-size driver — THE GUARDED-INLINE SIZE NUMBER.
#
# VERDICT.md gates the openspec proposal on this measurement: R5 (guarded
# inlining) and R1' (elision) both have emitted-size consequences that have to
# be priced against ADR 0004 before defexpand is proposed.
#
# `defexpand` does not exist in the compiler yet, so this does not measure a
# feature — it measures the three SHAPES a call site can take, by writing each
# one out longhand and compiling it for real. That is exactly what the
# analyzer would emit, so the sizes are the sizes.
#
#   defn      (defn add! [a x] (swap! a conj x))  + N calls        <- baseline
#   inline    the R1'-elided expansion spliced at N sites          <- R1..R4
#   guarded   (if (identical? add! add!--pristine) BODY (add! …))  <- R5
#
# Run from the spike dir:
#   BIN=<cljgo> WORK=<scratch> bash gen-size.sh
set -e
WORK=${WORK:?set WORK}
BIN=${BIN:?set BIN}
mkdir -p "$WORK/size"

gen() { # $1=kind $2=n
  local kind=$1 n=$2 f="$WORK/size/$kind-$n.clj" i
  {
    echo '(defn add! [a x] (swap! a conj x))'
    [ "$kind" = guarded ] && echo '(def add!--pristine add!)'
    echo '(def todo (atom []))'
    echo '(defn -main []'
    for i in $(seq 1 "$n"); do
      case $kind in
        # A bare call. `todo` and the literal are both dx-simple?, so R1'
        # elides every temporary and the three shapes stay comparable.
        defn)    echo "  (add! todo $i)" ;;
        inline)  echo "  (swap! todo conj $i)" ;;
        guarded) echo "  (if (identical? add! add!--pristine) (swap! todo conj $i) (add! todo $i))" ;;
      esac
    done
    echo '  (println (count @todo)))'
    echo '(-main)'
  } > "$f"
}

fsize() { stat -f%z "$1" 2>/dev/null || stat -c%s "$1"; }

printf '%-9s %5s %14s %14s\n' kind n main.go binary
for kind in defn inline guarded; do
  for n in 1 50 500; do
    gen "$kind" "$n"
    ( cd "$WORK/size" && "$BIN" build -gen "g-$kind-$n" -o "b-$kind-$n" "$kind-$n.clj" >/dev/null )
    lines=$(cat "$WORK/size/g-$kind-$n"/*.go | wc -l | tr -d ' ')
    printf '%-9s %5s %8s lines %9s bytes\n' "$kind" "$n" "$lines" "$(fsize "$WORK/size/b-$kind-$n")"
  done
done
