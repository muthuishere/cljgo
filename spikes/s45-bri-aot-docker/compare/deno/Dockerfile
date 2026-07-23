# syntax=docker/dockerfile:1
FROM denoland/deno:latest
WORKDIR /app
COPY server.ts ./
ENV PORT=8080
EXPOSE 8080
CMD ["run", "--allow-net", "--allow-env", "server.ts"]
