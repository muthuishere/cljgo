# newapp

A command-line tool in cljgo.

```
src/newapp/core.cljg        -main and the fns it calls
test/newapp/core_test.cljg  the test
build.cljgo                 the build plan: one exe artifact
```

## Work on it

```
cljgo test          # load src/, run every test under test/
cljgo build run     # compile and run it
cljgo build         # install ./newapp
./newapp ada alan   # Hello, ada, alan!
```

## Ship it

`cljgo build` produces a single static binary. Nothing to install
alongside it, no runtime on the far side, no classpath — copy the file
and run it. It starts in milliseconds, so it is at home in a git hook, a
CI step, or a shell pipeline.

## Grow it

- A library instead (no `-main`)? `cljgo new <name>` — that is the
  default.
- A web app? `cljgo new <name> --template web` — bri, cljgo's
  application framework (`docs/guides/bri-tutorial.md`).
