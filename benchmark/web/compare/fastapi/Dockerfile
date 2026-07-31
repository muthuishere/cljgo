# syntax=docker/dockerfile:1
FROM python:3.13-slim AS build
WORKDIR /app
COPY requirements.txt ./
RUN pip install --no-cache-dir --prefix=/install -r requirements.txt

FROM python:3.13-slim
WORKDIR /app
COPY --from=build /install /usr/local
COPY app.py ./
ENV PORT=8080
EXPOSE 8080
CMD ["python", "app.py"]
