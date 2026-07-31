# syntax=docker/dockerfile:1
# Stage 1: prefetch deps with the clojure CLI into a JVM builder image.
FROM clojure:temurin-26-tools-deps AS build
WORKDIR /app
COPY deps.edn ./
RUN clojure -P
COPY src ./src
RUN clojure -M -e "(require 'app.core)"

# Stage 2: carry the deps cache + source; run via clojure -M.
FROM clojure:temurin-26-tools-deps
WORKDIR /app
COPY --from=build /root/.m2 /root/.m2
COPY deps.edn ./
COPY src ./src
ENV PORT=8080
EXPOSE 8080
CMD ["clojure", "-M", "-m", "app.core"]
