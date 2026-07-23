# syntax=docker/dockerfile:1
FROM rust:1-slim AS build
WORKDIR /src
# Cache deps: build against a stub main, then swap in real sources.
COPY Cargo.toml ./
RUN mkdir src && echo 'fn main() {}' > src/main.rs \
    && cargo build --release 2>/dev/null || true
COPY src ./src
RUN touch src/main.rs && cargo build --release

FROM debian:stable-slim
COPY --from=build /src/target/release/server /server
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/server"]
