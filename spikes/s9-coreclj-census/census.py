#!/usr/bin/env python3
"""S9: census of upstream clojure/core.clj — classify top-level forms A/B/C.

A = pure Clojure, loads as-is given our special forms
B = needs host rewrite (Java interop present)
C = JVM-only / unreachable for cljgo (judgment-call list)
"""
import re
import sys
import json
from collections import defaultdict

# ---------------------------------------------------------------- splitter

def read_forms(src):
    """Paren-balanced top-level form splitter. Returns list of (start_line, text)."""
    forms = []
    i, n = 0, len(src)
    line = 1

    def skip_ws(i, line):
        while i < n:
            c = src[i]
            if c == ';':
                while i < n and src[i] != '\n':
                    i += 1
            elif c == '\n':
                line += 1
                i += 1
            elif c in ' \t\r,\f':
                i += 1
            elif c == '#' and i + 1 < n and src[i+1] == '!':
                while i < n and src[i] != '\n':
                    i += 1
            else:
                break
        return i, line

    def read_one(i, line):
        """Read one form starting at i (non-ws). Return (end_i, end_line)."""
        c = src[i]
        # prefix macros: read prefix then the following form
        if c in "'`~@^":
            i += 1
            if c == '~' and i < n and src[i] == '@':
                i += 1
            i, line = skip_ws(i, line)
            return read_one(i, line)
        if c == '#':
            nxt = src[i+1] if i + 1 < n else ''
            if nxt == '_':          # discard: read prefix + next form (kept in text)
                i += 2
                i, line = skip_ws(i, line)
                return read_one(i, line)
            if nxt == "'":
                i += 2
                i, line = skip_ws(i, line)
                return read_one(i, line)
            if nxt == '^':          # meta
                i += 2
                i, line = skip_ws(i, line)
                i, line = read_one(i, line)   # the meta map/kw
                i, line = skip_ws(i, line)
                return read_one(i, line)      # the target form
            if nxt == '"':          # regex
                i += 2
                while i < n:
                    if src[i] == '\\':
                        i += 2
                        continue
                    if src[i] == '"':
                        return i + 1, line
                    if src[i] == '\n':
                        line += 1
                    i += 1
                return i, line
            if nxt in '({[':
                pass  # fall through to delim reading below (skip the #)
            # #(...) #{...} #?(...) etc.: just advance past '#' and continue
            i += 1
            if i >= n:
                return i, line
            c = src[i]
        if c == '"':
            i += 1
            while i < n:
                if src[i] == '\\':
                    i += 2
                    continue
                if src[i] == '"':
                    return i + 1, line
                if src[i] == '\n':
                    line += 1
                i += 1
            return i, line
        if c == '\\':               # char literal
            i += 1
            # named chars: newline, space, tab, formfeed, backspace, return, uXXXX, oNNN
            m = re.match(r'(newline|space|tab|formfeed|backspace|return|u[0-9a-fA-F]{4}|o[0-7]{1,3})',
                         src[i:i+9])
            if m and (i + len(m.group(0)) >= n or not src[i+len(m.group(0))].isalnum()):
                return i + len(m.group(0)), line
            return i + 1, line
        if c in '([{':
            close = {'(': ')', '[': ']', '{': '}'}[c]
            i += 1
            while i < n:
                i, line = skip_ws(i, line)
                if i >= n:
                    break
                if src[i] == close:
                    return i + 1, line
                if src[i] in ')]}':   # mismatched; bail
                    return i + 1, line
                i, line = read_one(i, line)
            return i, line
        # symbol/keyword/number token
        while i < n and src[i] not in ' \t\r\n,()[]{}";':
            i += 1
        return i, line

    while True:
        i, line = skip_ws(i, line)
        if i >= n:
            break
        start, start_line = i, line
        i, line = read_one(i, line)
        forms.append((start_line, src[start:i]))
    return forms

# ---------------------------------------------------------------- tokenizer

STRIP_STR = re.compile(r'"(?:\\.|[^"\\])*"')
STRIP_COMMENT = re.compile(r';[^\n]*')
STRIP_CHAR = re.compile(r'\\(?:newline|space|tab|formfeed|backspace|return|u[0-9a-fA-F]{4}|o[0-7]{1,3}|.)')
TOKEN = re.compile(r"[^\s,()\[\]{}'`~@^]+")

def clean(text):
    t = STRIP_COMMENT.sub(' ', STRIP_STR.sub(' "" ', text))
    t = STRIP_CHAR.sub(' ', t)
    return t

def tokens(text):
    return TOKEN.findall(clean(text))

DEF_HEADS = {'def', 'defn', 'defn-', 'defmacro', 'defmacro-', 'definline',
             'defonce', 'defmulti', 'defmethod', 'defprotocol', 'defrecord',
             'deftype', 'definterface', 'defstruct'}

def head_and_name(text):
    """(head, defined-name) for a top-level form; name None if not a def."""
    t = clean(text).lstrip()
    m = re.match(r'\(\s*([^\s()\[\]{}]+)', t)
    if not m:
        return None, None
    head = m.group(1)
    if head not in DEF_HEADS:
        return head, None
    rest = t[m.end():]
    # skip metadata ^... (possibly several)
    while True:
        rest = rest.lstrip()
        if rest.startswith('^'):
            if rest[1:2] == '{':
                depth = 0
                j = 1
                while j < len(rest):
                    if rest[j] == '{':
                        depth += 1
                    elif rest[j] == '}':
                        depth -= 1
                        if depth == 0:
                            j += 1
                            break
                    j += 1
                rest = rest[j:]
            else:
                m2 = re.match(r'\^[^\s()\[\]{}]+', rest)
                rest = rest[m2.end():]
        else:
            break
    m3 = re.match(r'[^\s()\[\]{}]+', rest)
    return head, (m3.group(0) if m3 else None)

# ---------------------------------------------------------------- classify

# C: JVM-only / unreachable for cljgo — by defined name
C_NAMES = {
    'bean', 'memfn', 'resultset-seq', 'enumeration-seq',
    'class', 'class?', 'bases', 'supers', 'cast',
    'definterface', 'gen-class', 'gen-interface',
    'proxy', 'proxy-super', 'init-proxy', 'update-proxy', 'get-proxy-class',
    'construct-proxy', 'proxy-mappings', 'proxy-call-with-super', 'proxy-name',
    'get-super-and-interfaces',
    'add-classpath', 'compile', 'load-reader', 'clear-agent-errors',
    'agent-errors', 'await1',
    'monitor-enter', 'monitor-exit',
    'primitives-classnames', 'munge', 'namespace-munge',
    'Throwable->map', 'StackTraceElement->vec',
    'macroexpand-1',  # no — keep. removed below
}
C_NAMES.discard('macroexpand-1')
C_NAME_PREFIXES = ('unchecked-',)

# forms whose head alone marks C
C_HEADS = {'gen-class', 'gen-interface', 'definterface'}

# B signals -----------------------------------------------------------------
PRIM_HINTS = {'long', 'longs', 'double', 'doubles', 'int', 'ints', 'float',
              'floats', 'short', 'shorts', 'byte', 'bytes', 'boolean',
              'booleans', 'char', 'chars', 'objects'}

JAVA_CLASS = re.compile(r'^(?:[a-z][\w$]*\.)+[A-Z][\w$]*$')     # java.lang.Foo
CLASS_SLASH = re.compile(r'^[A-Z][\w$.]*/.+$')                   # Foo/bar
CTOR = re.compile(r'^[A-Z][\w$.]*\.$')                           # Foo.
METHOD_CALL = re.compile(r'^\.[A-Za-z_]')                        # .foo
KNOWN_JAVA = re.compile(r'^(?:java|javax|clojure\.lang|jdk)\.')

def classify(text, name, head):
    toks = tokens(text)
    tokset = set(toks)

    # --- C by name / head
    if head in C_HEADS:
        return 'C', 'head:' + head
    if name:
        if name in C_NAMES:
            return 'C', 'name:' + name
        if any(name.startswith(p) for p in C_NAME_PREFIXES):
            return 'C', 'name:unchecked-*'

    reasons = []
    # --- B signals
    for t in toks:
        if METHOD_CALL.match(t):
            reasons.append('.method')
            break
    if '.' in tokset:
        reasons.append('dot-form')
    if 'new' in tokset:
        reasons.append('new')
    for t in toks:
        if KNOWN_JAVA.match(t):
            reasons.append('java-class')
            break
    else:
        for t in toks:
            if JAVA_CLASS.match(t) and not t.startswith('clojure.core'):
                reasons.append('java-class')
                break
    for t in toks:
        if CLASS_SLASH.match(t):
            reasons.append('Class/member')
            break
    for t in toks:
        if CTOR.match(t):
            reasons.append('Ctor.')
            break
    # primitive / class type hints
    ctext = clean(text)
    for m in re.finditer(r'\^([^\s()\[\]{}]+)', ctext):
        h = m.group(1)
        if h in PRIM_HINTS:
            reasons.append('^prim-hint')
            break
        if re.match(r'^[A-Z]', h) or KNOWN_JAVA.match(h):
            reasons.append('^Class-hint')
            break
    if head == 'definline':
        reasons.append('definline')
    if 'proxy' in tokset or 'reify' in tokset:
        reasons.append('proxy/reify')
    if 'gen-class' in tokset:
        reasons.append('gen-class')
    # set! on static: (set! (. Foo bar) ...) or (set! Foo/bar ...)
    if re.search(r'\(set!\s+\(\.', ctext) or re.search(r'\(set!\s+[A-Z][\w$.]*/', ctext):
        reasons.append('set!-static')

    if reasons:
        return 'B', ','.join(sorted(set(reasons)))
    return 'A', ''

# ---------------------------------------------------------------- main

def main():
    path = sys.argv[1]
    src = open(path).read()
    forms = read_forms(src)

    rows = []
    defined = {}   # name -> index of defining form (first def wins)
    for idx, (ln, text) in enumerate(forms):
        head, name = head_and_name(text)
        cls, why = classify(text, name, head)
        rows.append({'idx': idx, 'line': ln, 'head': head, 'name': name,
                     'cls': cls, 'why': why, 'len': len(text)})
        if name and name not in defined:
            defined[name] = idx

    # def→use graph: for each defined name, which forms use it (excluding its own def)
    uses = defaultdict(set)
    for idx, (ln, text) in enumerate(forms):
        for t in set(tokens(text)):
            base = t.lstrip("'`~@#^")
            if base in defined and defined[base] != idx:
                uses[base].add(idx)

    # transitive unlock: forms directly using a B/C name, plus forms using those forms' names, etc.
    # For each B/C-defined name, count of downstream forms that (transitively) depend on it.
    children = defaultdict(set)   # name -> names whose defining form uses it
    name_of_form = {}
    for r in rows:
        if r['name']:
            name_of_form[r['idx']] = r['name']
    for nm, users in uses.items():
        for u in users:
            if u in name_of_form:
                children[nm].add(name_of_form[u])

    def transitive_users(nm):
        seen, stack = set(), [nm]
        while stack:
            cur = stack.pop()
            for ch in children.get(cur, ()):
                if ch not in seen:
                    seen.add(ch)
                    stack.append(ch)
        seen.discard(nm)
        return seen

    json.dump({'rows': rows,
               'uses': {k: sorted(v) for k, v in uses.items()},
               'defined': defined},
              open(sys.argv[2], 'w'))

    # ---------- report to stdout
    total = len(rows)
    counts = defaultdict(int)
    lines_per = defaultdict(int)
    for r in rows:
        counts[r['cls']] += 1
        lines_per[r['cls']] += r['len']
    print(f"total top-level forms: {total}")
    for c in 'ABC':
        print(f"  {c}: {counts[c]:4d}  ({100*counts[c]/total:5.1f}%)   chars: {lines_per[c]}")

    # cumulative by position
    print("\ncumulative A%% by form position:")
    ca = cb = cc = 0
    for i, r in enumerate(rows, 1):
        if r['cls'] == 'A': ca += 1
        elif r['cls'] == 'B': cb += 1
        else: cc += 1
        if i % 100 == 0 or i == total:
            print(f"  first {i:4d}: A {100*ca/i:5.1f}%  B {100*cb/i:5.1f}%  C {100*cc/i:5.1f}%")

    # top B/C by direct + transitive unlock
    scored = []
    for r in rows:
        if r['cls'] in 'BC' and r['name']:
            direct = len(uses.get(r['name'], ()))
            trans = len(transitive_users(r['name']))
            scored.append((direct, trans, r['name'], r['cls'], r['why'], r['line']))
    scored.sort(reverse=True)
    print("\ntop 40 most-referenced B/C defs (direct uses, transitive dependents):")
    for direct, trans, nm, cls, why, ln in scored[:40]:
        print(f"  {direct:4d} direct / {trans:4d} transitive  [{cls}] {nm}  (line {ln}; {why})")

    # B reason histogram
    why_hist = defaultdict(int)
    for r in rows:
        if r['cls'] == 'B':
            for w in r['why'].split(','):
                why_hist[w] += 1
    print("\nB reason histogram:")
    for w, c in sorted(why_hist.items(), key=lambda kv: -kv[1]):
        print(f"  {c:4d}  {w}")

if __name__ == '__main__':
    main()
