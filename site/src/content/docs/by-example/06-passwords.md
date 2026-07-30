---
title: "Passwords"
sidebar:
  label: "6. Passwords"
  order: 6
---

Storing a password correctly is one function call. cljgo ships argon2id —
the modern standard — with salting handled for you.

## Start small: hash it, check it

```clojure
(require '[cljg.security :as sec])

(def h (sec/hash-password "correct horse battery staple"))

(sec/check-password "correct horse battery staple" h)
```

Output:

```
true
```

`hash-password` returns a self-describing hash string — safe to store in
your database. It is salted and slow-by-design, so it can't be reversed or
looked up in a rainbow table.

## Wrong password? Just false

```clojure
(sec/check-password "Tr0ub4dor&3" h)
```

Output:

```
false
```

No exception to catch, no timing games to worry about — `false` means no.

Never store the password itself. Never compare with `=`. These two
functions are the entire correct recipe.

## The rest of the toolbox

The same namespace covers the everyday crypto you reach for:

```clojure
(sec/sha256 "hello")        ; digest, as hex
(sec/uuid)                  ; a fresh v4 uuid
(sec/token)                 ; 64 chars of secure randomness
```

Output:

```
2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
b0fc0d3f-97af-42a5-90d2-e34f2ff4b7c2
0d9c2ad10bfa5c…  (different every run)
```

`sha256` is for fingerprinting data — **not** for passwords; that's what
`hash-password` is for.

There's also a keychain in here — `save-to-keychain` /
`get-from-keychain` store secrets in your OS's credential store (with an
encrypted-file fallback on servers). It gets its own page later in the
book.
