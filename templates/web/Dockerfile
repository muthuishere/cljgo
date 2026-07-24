# syntax=docker/dockerfile:1
#
# Deploy this bri app as a single static binary in a minimal image
# (ADR 0071). The build stage installs the cljgo compiler and AOT-compiles
# the app to one CGO_ENABLED=0 binary; the final stage is `scratch` holding
# only that binary + config + static assets. Typically single-digit MB.
#
#   docker build -t newapp .
#   docker run -p 3000:3000 newapp
#
# Bump CLJGO_VERSION to move to a newer cljgo (the app's go.mod pins the
# matching runtime, fetched from the Go module proxy — no repo checkout).
ARG CLJGO_VERSION=v0.6.0

FROM golang:1.26 AS build
ARG CLJGO_VERSION
RUN go install github.com/muthuishere/cljgo/cmd/cljgo@${CLJGO_VERSION}
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux cljgo build -o /server src/app/main.cljg

FROM scratch
COPY --from=build /server /server
COPY --from=build /app/conf.edn /conf.edn
COPY --from=build /app/public /public
# conf.edn defaults :port 3000; APP_* env overrides any key (secrets-are-env).
ENV APP_PROFILE=prod
EXPOSE 3000
ENTRYPOINT ["/server"]
