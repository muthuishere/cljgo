;; ADR 0054 decision 4: the JVM-only (import …) special form is unresolved on
;; cljgo — a loud file:line error, never a silent no-op. cljgo does not do Java.
(import (java.io File))
;; harness: eval — ADR 0054 dec 4: import is a JVM-only special form; cljgo errors loudly rather than silently ignoring it
;; expect-error: unable to resolve symbol: import
