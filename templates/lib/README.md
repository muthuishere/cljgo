# newapp

A cljgo library.

```
src/newapp/core.cljg        the code
test/newapp/core_test.cljg  the test
build.cljgo                 the build plan (a library declares no artifacts)
```

## Work on it

```
cljgo test    # load src/, run every test under test/
cljgo repl    # a REPL
```

## Grow it

- A command-line tool instead? `cljgo new <name> --template cli` — a
  `-main`, argument handling, and a build plan that produces one static
  binary.
- A web app? `cljgo new <name> --template web` — keel, cljgo's
  application framework: routes as data, config as one map, a styled
  page (`docs/guides/keel-tutorial.md`).

Both are templates over this same shape: plain namespaces, plain files,
nothing scanned.
