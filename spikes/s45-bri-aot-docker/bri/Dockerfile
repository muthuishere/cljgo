# syntax=docker/dockerfile:1
#
# s45 — the compiled bri hello app as a minimal scratch image (ADR 0071 dec 4,
# spike exit criterion 3). Mirrors the compare/ corpus Dockerfiles: a Go build
# stage → a scratch final holding ONLY the static binary (+ conf.edn).
#
# BUILD CONTEXT = the cljgo REPO ROOT (the app build replaces the runtime to
# the in-repo source; a template app instead pins a released cljgo — task 4):
#
#   docker build -f spikes/s45-bri-aot-docker/bri/Dockerfile \
#                -t bri-hello .            # from the repo root
#
FROM golang:1.26 AS build
WORKDIR /cljgo
COPY . .
# The cljgo compiler, then the bri app → one static CGO_ENABLED=0 binary.
RUN go build -o /usr/local/bin/cljgo ./cmd/cljgo
WORKDIR /cljgo/spikes/s45-bri-aot-docker/bri
RUN CGO_ENABLED=0 GOOS=linux cljgo build -o /server src/app/main.cljg

FROM scratch
COPY --from=build /server /server
COPY --from=build /cljgo/spikes/s45-bri-aot-docker/bri/conf.edn /conf.edn
# conf.edn defaults :port 8080; APP_* env overrides any key (secrets-are-env).
ENV APP_PROFILE=prod
EXPOSE 8080
ENTRYPOINT ["/server"]
