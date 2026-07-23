# syntax=docker/dockerfile:1
FROM node:24-alpine
WORKDIR /app
COPY server.js ./
ENV PORT=8080
EXPOSE 8080
CMD ["node", "server.js"]
