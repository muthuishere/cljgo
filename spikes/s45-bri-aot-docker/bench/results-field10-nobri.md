# s45 web benchmark — 2026-07-24 02:19

oha: duration=15s concurrency=50 warm=3s · one container at a time · Docker 29.4.0

| runtime | image | cold-start | / req/s | / p99 ms | /api req/s | /api p99 ms | peak RSS |
|---|--:|--:|--:|--:|--:|--:|--:|
| bun | 333MB | 28ms | 74798 | 1.65 | 83535 | 1.32 | 50.4MiB |
| clj-httpkit | 847MB | 1277ms | 82837 | 1.02 | 83669 | 1.02 | 353.1MiB |
| clj-ring-jetty | 858MB | 1659ms | 67786 | 1.52 | 67442 | 1.57 | 490.9MiB |
| deno | 277MB | 146ms | 89316 | 0.95 | 89099 | 0.94 | 21.3MiB |
| dotnet | 359MB | 172ms | 62792 | 2.07 | 67451 | 1.85 | 47.3MiB |
| fastapi | 220MB | 381ms | 8931 | 10.48 | 8948 | 10.49 | 38.2MiB |
| go | 7.62MB | 30ms | 66876 | 2.58 | 55769 | 3.01 | 16.4MiB |
| node | 228MB | 147ms | 55344 | 1.96 | 62167 | 1.6 | 133.6MiB |
| rust-axum | 140MB | 28ms | 89480 | 0.98 | 89986 | 0.96 | 8.2MiB |
| spring-boot | 512MB | 858ms | 51002 | 2.04 | 55056 | 1.66 | 573.9MiB |
