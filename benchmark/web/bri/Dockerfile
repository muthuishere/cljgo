# syntax=docker/dockerfile:1
#
# The compiled bri hello app as a minimal scratch image (ADR 0071). Mirrors the
# compare/ corpus Dockerfiles: a Go build stage → a scratch final holding ONLY
# the static binary (+ conf.edn).
#
# It builds cljgo FROM THIS CHECKOUT, so the number you get is the number your
# working tree produces — not a released binary. BUILD CONTEXT = the cljgo REPO
# ROOT:
#
#   docker build -f benchmark/web/bri/Dockerfile -t bri-hello .
#
# run.sh does this for you.
#
FROM golang:1.26 AS build
WORKDIR /cljgo
COPY . .
# The cljgo compiler, then the bri app → one static CGO_ENABLED=0 binary.
RUN go build -o /usr/local/bin/cljgo ./cmd/cljgo
WORKDIR /cljgo/benchmark/web/bri
RUN CGO_ENABLED=0 GOOS=linux cljgo build -o /server src/app/main.cljg

FROM scratch
COPY --from=build /server /server
COPY --from=build /cljgo/benchmark/web/bri/conf.edn /conf.edn
# conf.edn defaults :port 8080; APP_* env overrides any key (secrets-are-env).
ENV APP_PROFILE=prod
EXPOSE 8080
ENTRYPOINT ["/server"]
