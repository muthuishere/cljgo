# syntax=docker/dockerfile:1
# Stage 1: prefetch deps with the clojure CLI into a JVM builder image.
FROM clojure:temurin-26-tools-deps AS build
WORKDIR /app
COPY deps.edn ./
# Download all dependencies into the image so runtime needs no network.
RUN clojure -P
COPY src ./src
# Warm compile check (also caches AOT of clojure runtime classes).
RUN clojure -M -e "(require 'app.core)"

# Stage 2: same base carries the deps cache + source; run via clojure -M.
FROM clojure:temurin-26-tools-deps
WORKDIR /app
COPY --from=build /root/.m2 /root/.m2
COPY deps.edn ./
COPY src ./src
ENV PORT=8080
EXPOSE 8080
CMD ["clojure", "-M", "-m", "app.core"]
