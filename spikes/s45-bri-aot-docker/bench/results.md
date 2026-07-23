# s45 web benchmark — 2026-07-24 02:42

oha: duration=15s concurrency=50 warm=3s · one container at a time · Docker 29.4.0

| runtime | image | cold-start | / req/s | / p99 ms | /api req/s | /api p99 ms | peak RSS |
|---|--:|--:|--:|--:|--:|--:|--:|
| bri | 15.5MB | 37ms | 21023 | 8.59 | 21866 | 8.76 | 33.5MiB |
| bun | 333MB | 35ms | 64752 | 2.26 | 66493 | 2.22 | 16.3MiB |
| clj-httpkit | 847MB | 1895ms | 74378 | 1.2 | 72328 | 1.23 | 323.8MiB |
| clj-ring-jetty | 858MB | 1791ms | 61505 | 1.67 | 64230 | 1.66 | 522.0MiB |
| deno | 277MB | 147ms | 70695 | 1.6 | 73272 | 1.56 | 44.6MiB |
| dotnet | 359MB | 173ms | 64004 | 1.91 | 66415 | 1.75 | 102.1MiB |
| fastapi | 220MB | 490ms | 7955 | 11.32 | 7451 | 12.18 | 38.3MiB |
| go | 7.62MB | 36ms | 69537 | 2.63 | 67891 | 2.73 | 15.5MiB |
| node | 228MB | 152ms | 42189 | 3.47 | 46058 | 3.24 | 43.0MiB |
