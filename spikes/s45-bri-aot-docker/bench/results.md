# s45 web benchmark — 2026-07-24 01:59

oha: duration=15s concurrency=50 warm=3s · one container at a time · Docker 29.4.0

| runtime | image | cold-start | / req/s | / p99 ms | /api req/s | /api p99 ms | peak RSS |
|---|--:|--:|--:|--:|--:|--:|--:|
| go | 7.62MB | 30ms | 69410 | 2.52 | 70239 | 2.64 | 17.2MiB |
| clj-ring-jetty | 858MB | 1754ms | 53525 | 1.62 | 55708 | 1.66 | 466.2MiB |
| clj-httpkit | 847MB | 1173ms | 61105 | 1.44 | 55764 | 1.58 | 336.2MiB |
| bun | 333MB | 32ms | 69940 | 1.77 | 74423 | 1.56 | 51.7MiB |
| node | 228MB | 145ms | 44658 | 2.92 | 56601 | 1.87 | 134.0MiB |
| deno | 277MB | 149ms | 81586 | 1.06 | 84122 | 1.1 | 17.9MiB |
