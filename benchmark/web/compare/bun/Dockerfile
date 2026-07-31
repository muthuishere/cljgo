# syntax=docker/dockerfile:1
FROM oven/bun:1
WORKDIR /app
COPY server.js ./
ENV PORT=8080
EXPOSE 8080
CMD ["bun", "run", "server.js"]
