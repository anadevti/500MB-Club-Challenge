# 500MB Club Challenge

> API de telemetria IoT de alta performance sob restrição de 2 CPUs / 500 MB de RAM.

**Go** | **Redis** | **Nginx** | **Docker**

## Arquitetura

```
                          ┌─────────────┐
                     ┌───→│  api-1:8000  │───┐
┌──────┐   ┌──────┐ │    └─────────────┘    │    ┌─────────────┐
│  k6  │──→│ nginx│──┤    ┌─────────────┐    ├───→│ redis:6379  │
└──────┘   │ :8080│  ├───→│  api-2:8000  │───┤    └─────────────┘
           └──────┘  │    └─────────────┘    │
                     │    ┌─────────────┐    │
                     └───→│  api-3:8000  │───┘
                          └─────────────┘
```

### Resource Budget

| Service   | CPU      | RAM      |
|-----------|----------|----------|
| nginx     | 0.10     | 32M      |
| redis     | 0.40     | 192M     |
| api-1     | 0.50     | 92M      |
| api-2     | 0.50     | 92M      |
| api-3     | 0.50     | 92M      |
| **Total** | **2.00** | **500M** |

## Quick Start

```bash
docker compose build
docker compose up -d
curl http://localhost:8080/readyz
```

## API

| Method | Path                                               | Status                                            |
|--------|----------------------------------------------------|---------------------------------------------------|
| `GET`  | `/healthz`                                         | `200` liveness probe                              |
| `GET`  | `/readyz`                                          | `200` / `503` readiness probe                     |
| `GET`  | `/metrics`                                         | `200` Prometheus metrics                          |
| `POST` | `/devices/{id}/telemetry`                          | `202` ingere 1 ponto                              |
| `POST` | `/devices/{id}/telemetry/batch`                    | `202` ingere 1-100 pontos                         |
| `GET`  | `/devices/{id}/telemetry?from=&to=&limit=&cursor=` | `200` query por janela temporal                   |
| `GET`  | `/devices/{id}/anomaly`                            | `200` / `404` z-score dos ultimos 256 pontos      |

## Decisoes de Design

- **Redis sorted sets** com `score = timestamp` para write O(log N) e range query O(log N + M)
- **Persistencia desligada** no Redis (sem AOF, sem RDB) para maximo throughput
- **`scratch` como base image** resultando em binario estatico de ~8 MB
- **Graceful shutdown** com draining de 10s via SIGTERM
- **Cursor-based pagination** com exclusive range no `ZRANGEBYSCORE`
- **Anomaly z-score** computado on-the-fly sem cache, conforme regra do desafio

## Testes

Testes unitarios cobrindo handlers, models, middleware e router.

```bash
go test ./...
```

Os testes foram escritos com auxilio do [Claude Code](https://claude.ai/claude-code).

## Licenca

[MIT](LICENSE)
