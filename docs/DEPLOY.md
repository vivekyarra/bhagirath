# Deploying

The app is one container (`Dockerfile`, a ~20 MB distroless image) plus PostgreSQL 16. It migrates on start, retries the database for up to 90 s, and serves `/healthz` for health checks. Schedulers hold a database lease, so running several replicas is safe.

## Render (blueprint)

1. Render → **New → Blueprint** → choose this repository. `render.yaml` creates the web service and a Postgres database.
2. Set `BHAGIRATH_BASE_URL` to the service URL, and `BHAGIRATH_JUDGE_PASSWORD` for the demo.
3. Open `/healthz`, then `/`. In demo mode, fictional seed data loads on first start.

## Fly.io

```bash
fly launch --no-deploy
```

```bash
fly postgres create --name bhagirath-db && fly postgres attach bhagirath-db
```

```bash
fly secrets set BHAGIRATH_SECRET_KEY="$(openssl rand -base64 48)" BHAGIRATH_BASE_URL=https://<app>.fly.dev BHAGIRATH_JUDGE_PASSWORD=<choose>
```

```bash
fly deploy
```

## Any Docker host

```bash
docker compose up -d --build
```

Put it behind a TLS-terminating proxy (Caddy, nginx or a cloud load balancer) that sets `X-Forwarded-Proto: https`.

## Two deployments, two purposes

| | Demo (for judges) | Pilot (real residents) |
|---|---|---|
| `BHAGIRATH_MODE` | `demo` | `pilot` |
| Data | fictional seed, resettable | real; back up the database daily |
| Messages | recorded as "simulated", never sent | sent through the configured providers |
| Simulator and clock jump | on | off |
| Ladder | `config/ladder.example.yaml` | `config/ladder.local.yaml` (git-ignored) |
| Required | — | `BHAGIRATH_SECRET_KEY`, a 12+ character admin password, a real ladder file, `BHAGIRATH_ADVISORY_REVIEWED_BY` before advisories |

## Uptime during judging

- Keep at least one machine always running (`min_machines_running = 1` on Fly; a paid instance on Render). Timers and callbacks run in-process.
- Point an external monitor (UptimeRobot, Better Stack) at `/healthz` every minute, then record the uptime percentage on submission day and once afterwards (spec §16).
- Free Postgres tiers can expire. Check the expiry date against the 5 December 2026 results date.
