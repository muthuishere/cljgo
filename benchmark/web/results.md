# s45 web benchmark — 2026-08-02 11:45

oha: duration=20s concurrency=50 warm=3s · one container at a time · Docker 29.4.0

| runtime | image | cold-start | / req/s | / p99 ms | /api req/s | /api p99 ms | peak RSS |
|---|--:|--:|--:|--:|--:|--:|--:|
| bri | 20.1MB | 29ms | 39263 | 4.42 | 38206 | 4.56 | 34.7MiB |
| bun | 333MB | 138ms | 78152 | 1.55 | 76165 | 1.63 | 16.6MiB |
| clj-httpkit | 847MB | 1364ms | 74170 | 0.97 | 75941 | 0.99 | 321.7MiB |
| clj-ring-jetty | 858MB | 1656ms | 56500 | 1.6 | 56757 | 1.63 | 440.2MiB |
| deno | 277MB | 258ms | 80101 | 1.09 | 75230 | 1.39 | 41.7MiB |
| dotnet | 359MB | 169ms | 64831 | 1.53 | 66015 | 1.62 | 46.7MiB |
| fastapi | 220MB | 480ms | 9529 | 9.15 | 9616 | 8.34 | 41.0MiB |
| go | 7.62MB | 26ms | 66922 | 2.26 | 66425 | 2.33 | 14.7MiB |
| node | 228MB | 59ms | 61973 | 1.57 | 61782 | 1.57 | 71.1MiB |
| rust-axum | 140MB | 27ms | 78957 | 0.92 | 79878 | 0.92 | 8.6MiB |
| spring-boot | 512MB | 958ms | 69814 | 1.31 | 69722 | 1.28 | 660.5MiB |
