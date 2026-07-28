---
title: "Which state tool?"
sidebar:
  label: "Which state tool?"
  order: 4
---

"State" covers four different problems that people confuse. Sorting them
out takes two questions:

**Does it have to survive a restart?** If yes, it's a database. Nothing
else on this page will save you.

**If not — do you need the answer now, or later?** Now → an atom or a
cache. Later → a job queue.

## Start small: an atom

An atom is a box holding one value, in memory, right now. It's the
default. Reach for it first and only move on when it stops being enough.

```clojure
(def hits (atom 0))

(swap! hits inc)
(swap! hits inc)

(println @hits)
```

Output:

```
2
```

Safe across goroutines, no locks to think about, gone when the process
exits. Atoms get a [whole page of their own](/cljgo/by-example/04-state-with-atoms/).

## A cache: an atom that forgets, and doesn't repeat work

Use a cache when the value is *expensive to compute* and *fine to be
slightly stale* — a user record, a currency rate, a rendered page.

```clojure
(require '[cljg.cache :as cache])

(def c (cache/local {:ttl 60}))

(defn load-user [id]
  (println "hitting the database for" id)
  {:id id :name "Asha"})

(println (cache/fetch c 42 #(load-user 42)))
(println (cache/fetch c 42 #(load-user 42)))
```

Output:

```
hitting the database for 42
{:id 42, :name Asha}
{:id 42, :name Asha}
```

Two fetches, one database hit. `fetch` takes the key and a function to
call *on a miss*; `:ttl 60` means entries expire after sixty seconds.

The part you can't see is the important one. If a thousand requests miss
the same key at the same moment, the function still runs **once** — the
rest wait for that one answer. That's called singleflight, and it's the
difference between a cache and a map with timestamps. It's also why you
should not build this yourself with an atom.

## A queue: work that shouldn't block the answer

Use a queue when the caller doesn't need the result — sending a welcome
email, resizing an upload, warming a cache. Hand it off and reply
immediately.

```clojure
(require '[cljg.jobs :as jobs])

(def q
  (jobs/local
    {:email/welcome (fn [{:keys [to]}]
                      (println "sending welcome mail to" to))}))

(jobs/submit q :email/welcome {:to "asha@example.com"})
(jobs/submit q :email/welcome {:to "ravi@example.com"})

(jobs/drain q)
(jobs/stop q)

(println "done")
```

Output:

```
sending welcome mail to asha@example.com
sending welcome mail to ravi@example.com
done
```

You register handlers by name up front, then `submit` a job type and a
payload. `submit` returns immediately — a pool of workers picks the job
up. Several workers run at once, so don't count on the order.

`drain` waits for everything outstanding to finish, which is mostly a
testing tool: in a real server you submit and move on. And because these
jobs live in memory, a crash loses them — if losing one would matter, the
job belongs in a database.

## A database: the only thing that survives

When the data must outlive the process, write it down.

```clojure
(require '[cljg.data.cast :as db])

(def conn (db/connect {:driver :sqlite :url "file:notes.db"}))

(db/exec! conn "create table if not exists notes (id integer primary key, body text)")
(db/exec! conn "insert into notes (body) values (?)" "remember the milk")

(println (db/query conn "select id, body from notes"))

(db/close! conn)
```

Output:

```
[{:id 1, :body remember the milk}]
```

SQL as a string, values as separate `?` parameters — never glued into the
string, which is what makes SQL injection structurally impossible here.
Rows come back as ordinary maps. The same API drives Postgres: change
`:driver`.

## Put it together: the decision

| what you have | use |
|---|---|
| a counter, a registry, a bit of app state | an **atom** |
| something slow to compute, fine slightly stale | **`cljg.cache`** |
| work the caller shouldn't wait for | **`cljg.jobs`** |
| anything that must survive a restart | **`cljg.data.cast`** |
| slow *and* must survive | the database, with a cache in front |

They stack rather than compete. A typical request handler reads through a
cache, which reads through the database, and submits a job on the way
out. Three tools, three jobs, each one boring on its own — which is
exactly what you want from state.
