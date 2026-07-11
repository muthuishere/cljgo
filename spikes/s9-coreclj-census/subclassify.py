#!/usr/bin/env python3
"""S9 part 2: sub-classify B forms (rt-only vs host-java vs mixed) and
compare upstream core.clj vs Glojure's rewritten core.glj form-by-form."""
import re, sys, json, difflib
from collections import defaultdict
from census import read_forms, tokens, head_and_name, clean, classify, PRIM_HINTS

KNOWN_RT = re.compile(r'^clojure\.lang\.')
KNOWN_JAVA = re.compile(r'^(?:java|javax|jdk)\.')
JAVA_CLASS = re.compile(r'^(?:[a-z][\w$]*\.)+[A-Z][\w$]*$')
CLASS_SLASH = re.compile(r'^([A-Z][\w$.]*)/.+$')
CTOR = re.compile(r'^([A-Z][\w$.]*)\.$')

# Bare capitalized class names that are java.lang.* (imported by default on JVM)
JAVA_LANG_BARE = {'String', 'Object', 'Boolean', 'Character', 'Class', 'Long',
    'Integer', 'Double', 'Float', 'Short', 'Byte', 'Number', 'Math', 'System',
    'Thread', 'Throwable', 'Exception', 'RuntimeException', 'Error',
    'IllegalArgumentException', 'IllegalStateException', 'ClassCastException',
    'UnsupportedOperationException', 'IndexOutOfBoundsException',
    'ArithmeticException', 'NullPointerException', 'StringBuilder',
    'CharSequence', 'Comparable', 'Iterable', 'Runnable', 'Callable',
    'BigDecimal', 'BigInteger', 'Character$UnicodeBlock', 'StackTraceElement'}

def java_flavor(text):
    """Return set of interop 'flavors' used by a form: {'rt','java'}"""
    flav = set()
    for t in tokens(text):
        base = t.lstrip("'`~@#^")
        if KNOWN_RT.match(base):
            flav.add('rt'); continue
        if KNOWN_JAVA.match(base):
            flav.add('java'); continue
        m = CLASS_SLASH.match(base) or CTOR.match(base)
        if m:
            cls = m.group(1)
            if KNOWN_RT.match(cls): flav.add('rt')
            elif cls in JAVA_LANG_BARE or KNOWN_JAVA.match(cls) or '.' not in cls:
                flav.add('java')
            continue
        if JAVA_CLASS.match(base):
            flav.add('java'); continue
        if base in JAVA_LANG_BARE:
            flav.add('java')
    # hints
    for m in re.finditer(r'\^([^\s()\[\]{}]+)', clean(text)):
        h = m.group(1)
        if KNOWN_RT.match(h): flav.add('rt')
        elif h in JAVA_LANG_BARE or KNOWN_JAVA.match(h): flav.add('java')
        elif h in PRIM_HINTS: flav.add('prim')
    return flav

def norm(text):
    """whitespace-normalized form text for diffing"""
    return re.sub(r'\s+', ' ', text).strip()

def index_forms(path):
    forms = read_forms(open(path).read())
    by_key = {}
    order = []
    for ln, text in forms:
        head, name = head_and_name(text)
        key = (head, name) if name else None
        order.append((key, ln, text))
        if key and key not in by_key:
            by_key[key] = text
    return order, by_key

def main():
    up_path = sys.argv[1]      # upstream (glojure originals for fair diff)
    glj_path = sys.argv[2]     # glojure core.glj
    census_src = sys.argv[3]   # downloads master core.clj (census target)

    # ---- B sub-classification on census target
    forms = read_forms(open(census_src).read())
    sub = defaultdict(int)
    sub_first500 = defaultdict(int)
    b_host_names = []
    for i, (ln, text) in enumerate(forms):
        head, name = head_and_name(text)
        cls, why = classify(text, name, head)
        if cls != 'B':
            key = cls
        else:
            fl = java_flavor(text)
            if fl <= {'rt'}:
                key = 'B-rt'          # only clojure.lang.* → mechanical
            elif fl <= {'rt', 'prim'}:
                key = 'B-rt'
            elif 'java' in fl:
                key = 'B-host'        # real java.* host surface
            else:
                key = 'B-other'       # dot-forms/new on locals, prim hints only
        sub[key] += 1
        if i < 500:
            sub_first500[key] += 1
        if key == 'B-host' and name:
            b_host_names.append((ln, name))
    total = len(forms)
    print("=== B sub-classification (census on downloads master) ===")
    for k in ('A', 'B-rt', 'B-other', 'B-host', 'C'):
        print(f"  {k:8s}: {sub[k]:4d}  ({100*sub[k]/total:5.1f}%)   first500: {sub_first500[k]:4d} ({100*sub_first500[k]/500:5.1f}%)")

    # ---- Glojure comparison
    up_order, up = index_forms(up_path)
    glj_order, glj = index_forms(glj_path)
    up_keys = set(up); glj_keys = set(glj)
    common = up_keys & glj_keys
    identical = sum(1 for k in common if norm(up[k]) == norm(glj[k]))
    changed = [k for k in common if norm(up[k]) != norm(glj[k])]
    dropped = up_keys - glj_keys
    added = glj_keys - up_keys
    print(f"\n=== Glojure vs upstream (matched by (head,name)) ===")
    print(f"  upstream named forms : {len(up_keys)}")
    print(f"  glojure named forms  : {len(glj_keys)}")
    print(f"  common               : {len(common)}")
    print(f"  byte-identical (ws-normalized): {identical} ({100*identical/len(common):.1f}% of common)")
    print(f"  changed              : {len(changed)}")
    print(f"  dropped by glojure   : {len(dropped)}")
    print(f"  added by glojure     : {len(added)}")
    print("\n  dropped:", sorted(str(k[1]) for k in dropped))
    print("\n  added:", sorted(str(k[1]) for k in added))

    # change magnitude for changed forms
    print("\n=== change magnitude (similarity ratio) for changed forms ===")
    ratios = []
    for k in changed:
        r = difflib.SequenceMatcher(None, norm(up[k]), norm(glj[k])).ratio()
        ratios.append((r, k))
    ratios.sort()
    buckets = defaultdict(int)
    for r, k in ratios:
        if r >= 0.9: buckets['>=90% similar'] += 1
        elif r >= 0.7: buckets['70-90%'] += 1
        elif r >= 0.4: buckets['40-70%'] += 1
        else: buckets['<40% (rewritten)'] += 1
    for b, c in buckets.items():
        print(f"  {b}: {c}")

    json.dump({'changed': [[list(map(str,k)), r] for r, k in ratios]},
              open('glojure-diff.json', 'w'))

    # sample diffs: pick ~20 spread across similarity spectrum
    print("\n=== 20 sample diffs (upstream → glojure) ===")
    step = max(1, len(ratios) // 20)
    for r, k in ratios[::step][:20]:
        u, g = norm(up[k]), norm(glj[k])
        print(f"\n--- {k[1]} (sim {r:.2f}) ---")
        # print first differing region, compact
        sm = difflib.SequenceMatcher(None, u, g)
        shown = 0
        for tag, i1, i2, j1, j2 in sm.get_opcodes():
            if tag != 'equal' and shown < 3:
                print(f"  - {u[max(0,i1-30):i2+30][:160]}")
                print(f"  + {g[max(0,j1-30):j2+30][:160]}")
                shown += 1

if __name__ == '__main__':
    main()
