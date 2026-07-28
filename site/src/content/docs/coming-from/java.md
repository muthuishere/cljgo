---
title: "Coming from Java"
sidebar:
  label: "From Java"
  order: 2
---

Clojure grew up on the JVM, so a lot of this will feel like home: the same
`clojure.string`, `clojure.set`, `clojure.walk`, the same lazy sequences,
the same `defrecord`. cljgo runs that language on Go instead.

What you gain is the packaging story. No JVM to install, no fat jar, no
warm-up: one static binary that starts in single-digit milliseconds. What
you give up is `import java.*` — the host underneath is Go.

## Start small: a POJO is a map

Java:

```java
public class User {
    private final String name;
    private final int age;
    // constructor, getName(), getAge(), equals, hashCode, toString...
}
```

cljgo:

```clojure
(def user {:name "Asha" :age 30})

(println (:name user))
(println (:age user))
```

Output:

```
Asha
30
```

That map already has structural equality, a sensible `toString`, and a
hash code — the things Lombok or a `record` generate for you. `:name` is a
keyword, and a keyword is also a function of a map, so `(:name user)` is
the getter.

## Values are final, all the way down

Java's `final` stops the *reference* from moving. Here the *value itself*
never changes:

```clojure
(def user {:name "Asha" :age 30})

(def older (assoc user :age 31))

(println older)
(println user)
```

Output:

```
{:name Asha, :age 31}
{:name Asha, :age 30}
```

`assoc` returns a new map and leaves the old one alone. Internally they
share structure, so this is cheap — it is not `new HashMap<>(old)`.

Practically: defensive copies stop being a thing, and any value is safe to
hand to another thread.

## Streams are just functions on sequences

Java:

```java
int total = nums.stream()
                .filter(n -> n % 2 == 0)
                .map(n -> n * n)
                .reduce(0, Integer::sum);
```

cljgo:

```clojure
(def nums [1 2 3 4 5 6])

(println (->> nums
              (filter even?)
              (map #(* % %))
              (reduce +)))
```

Output:

```
56
```

`->>` is the pipeline: it threads the value through each step as the last
argument, so you read top to bottom exactly like `.stream()`. And
`#(* % %)` is a lambda — `%` is its argument.

There's no `.collect(...)` at the end because there was never a stream
object to close. `map` and `filter` return **lazy sequences**, and `reduce`
simply consumes one.

Lazy means infinite is allowed:

```clojure
(def squares (map #(* % %) (range)))

(println (take 5 squares))
```

Output:

```
(0 1 4 9 16)
```

`(range)` counts forever. Nothing computed until `take` asked for five.

## Checked exceptions become data

Java makes you declare `throws`. Clojure throws one thing — `ex-info` —
and what makes it useful is the map it carries:

```clojure
(defn charge! [amount]
  (when (neg? amount)
    (throw (ex-info "amount must be positive"
                    {:amount amount :code :bad-amount})))
  (str "charged " amount))

(println (charge! 500))

(try
  (charge! -20)
  (catch Exception e
    (println "failed:" (ex-message e))
    (println "details:" (:code (ex-data e)))))
```

Output:

```
charged 500
failed: amount must be positive
details: :bad-amount
```

Instead of a class hierarchy per failure mode, you get `ex-data` — a plain
map you can branch on, log as JSON, or pass across a boundary. One
exception type, arbitrary detail.

The trailing `!` in `charge!` is just a naming convention: "this one has an
effect".

## The standard library you already know

`clojure.*` namespaces behave the way they do on the JVM:

```clojure
(require '[clojure.string :as str])

(println (str/upper-case "hello"))
(println (str/join ", " ["a" "b" "c"]))
(println (str/split "a,b,c" #","))
```

Output:

```
HELLO
a, b, c
[a b c]
```

Alongside them sits a `cljg.*` family — HTTP, compression, crypto,
sockets, jobs — that fills the role `java.net`, `java.util.zip` and
`javax.crypto` play for you today.

## Put it together: group and sum, without a Collector

The Java version of this is `Collectors.groupingBy(..., summingInt(...))`.
Here it's the same two ideas spelled out:

```clojure
(def staff
  [{:name "Asha" :dept "eng"   :salary 120}
   {:name "Ravi" :dept "eng"   :salary 100}
   {:name "Mina" :dept "sales" :salary  90}])

(defn payroll [people]
  (->> people
       (group-by :dept)
       (map (fn [[dept members]]
              [dept (reduce + (map :salary members))]))
       (into {})))

(println (payroll staff))
```

Output:

```
{eng 220, sales 90}
```

`group-by :dept` builds a map from department to the people in it. Then
each `[dept members]` pair is destructured, summed, and `into {}` collects
the pairs back into a map. Four lines, no `Collector`, no type parameters.

## Build and ship

Maven and Gradle become one file, `build.cljgo`, written in the same
language as the app:

```clojure
(defn build [b]
  (let [app (exe b {:name "newapp"
                    :main "src/newapp/core.cljg"})]
    (install b app)
    (run b app)))
```

```bash
cljgo run hello.clj              # no build step at all
cljgo build -o hello hello.clj   # one static binary
./hello
```

Output:

```
Hello from a static binary!
```

No `java -jar`, no JRE on the target box, no cold-start penalty in a
lambda. Copy the file and run it.

Ready for the language itself? Start at **1. Hello World** — it assumes no
Clojure at all, and moves fast.
