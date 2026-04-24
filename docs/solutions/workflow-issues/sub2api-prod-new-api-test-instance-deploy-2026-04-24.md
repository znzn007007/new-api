---
title: Deploy a parallel new-api test instance on sub2api-prod by reusing PostgreSQL and Redis
date: 2026-04-24
category: workflow-issues
module: sub2api-prod deployment
problem_type: workflow_issue
component: tooling
severity: medium
applies_when:
  - You need to validate current new-api repo code on sub2api-prod without replacing the production container
  - The server already has running postgres and redis containers that should be reused
  - You want a reversible test deployment on a separate port
  - Local Docker is unavailable or unreliable and a server-side build fallback is needed
tags: [new-api, sub2api-prod, docker-compose, test-deploy, postgres, redis, remote-build, gcp]
symptoms:
  - Production new-api must stay online while testing current repo code
  - Local Docker cannot reliably build the image
  - A temporary test port is needed without rebuilding postgres or redis
root_cause: missing_tooling
resolution_type: workflow_improvement
---

# Deploy a parallel new-api test instance on sub2api-prod by reusing PostgreSQL and Redis

## Context
The goal was to validate the current `new-api` repository on `sub2api-prod` without touching the production container running on `127.0.0.1:3000`. The machine already had stable `postgres` and `redis` containers, so the safest path was a second `new-api` container with its own deploy directory, its own `data/` and `logs/`, and a new loopback port.

During execution, local Docker on the workstation was unavailable, so the workflow needed a fallback that still produced a test image and kept the rollout reversible.

## Guidance
Use a **parallel test instance** instead of replacing production:

1. Keep production `new-api` unchanged.
2. Reuse the existing Docker network and the existing `postgres` / `redis` containers.
3. Create a separate deploy directory such as `/opt/new-api-test`.
4. Run the test container on `127.0.0.1:3001:3000`.
5. Give the test container its own bind mounts:
   - `/opt/new-api-test/data:/data`
   - `/opt/new-api-test/logs:/app/logs`
6. Reuse production connection settings:
   - `SQL_DSN`
   - `REDIS_CONN_STRING`
   - `SESSION_SECRET`
7. Run the test instance in a conservative mode:
   - `NODE_TYPE=slave`
   - `UPDATE_TASK=false`
   - `BATCH_UPDATE_ENABLED=false`
   - `CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED=false`

When local Docker cannot build, use a **server-side build fallback**:

1. Archive the current repo state with `git archive`.
2. Upload the archive to the server with `gcloud compute scp`.
3. Extract it under `/tmp/...`.
4. Build the image on the server with `docker build`.
5. Deploy with a dedicated `docker-compose.yml` under `/opt/new-api-test`.

The resulting test deployment on `sub2api-prod` used:

- image: `new-api-local:test-20260424-051402-96d99d14`
- container: `new-api-test`
- network: `new-api_new-api-network`
- port: `127.0.0.1:3001:3000`

## Why This Matters
This pattern gives a safe validation lane:

- production keeps serving traffic on `3000`
- test traffic is isolated to `3001`
- rollback is trivial: stop the test container
- `postgres` and `redis` remain untouched
- logs and writable state are separated from production

It also provides a practical fallback when the operator machine cannot run Docker locally. That avoids blocking deployment testing on workstation-specific tooling failures.

One important caveat: this is **not a fully isolated environment**. Reusing production `postgres` and `redis` means data-layer effects can still leak across environments. The setup is suitable for careful smoke testing and deployment verification, not for destructive experiments.

## When to Apply
- You need a reversible smoke-test deployment on an existing production VM
- Production already uses Docker Compose and shared services are stable
- You only need a second application container, not a second database/cache stack
- You want to test over SSH tunnel first instead of immediately opening a public firewall port

## Examples
### Test instance compose shape

```yaml
version: '3.4'
services:
  new-api-test:
    image: new-api-local:test-20260424-051402-96d99d14
    container_name: new-api-test
    restart: always
    command: --log-dir /app/logs
    ports:
      - "127.0.0.1:3001:3000"
    volumes:
      - ./data:/data
      - ./logs:/app/logs
    env_file:
      - .env.test
    networks:
      - existing-network

networks:
  existing-network:
    external: true
    name: new-api_new-api-network
```

### Test instance env shape

```env
SQL_DSN=postgresql://newapi:***@postgres:5432/new_api?sslmode=disable
REDIS_CONN_STRING=redis://:***@redis:6379
SESSION_SECRET=***
TZ=Asia/Shanghai
NODE_TYPE=slave
NODE_NAME=new-api-test
ERROR_LOG_ENABLED=true
UPDATE_TASK=false
BATCH_UPDATE_ENABLED=false
CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED=false
GENERATE_DEFAULT_TOKEN=false
PGSSLMODE=disable
```

### Verification checklist

```bash
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}'
curl -s http://127.0.0.1:3000/api/status
curl -s http://127.0.0.1:3001/api/status
curl -i http://127.0.0.1:3001/v1/models
docker logs --tail 120 new-api-test
```

Observed result on 2026-04-24:

- production stayed on `calciumion/new-api:v0.12.11`
- test instance responded successfully on `127.0.0.1:3001`
- `/v1/models` on the test instance returned `401`, confirming auth behavior remained intact
- `enable_batch_update` was `false` on the test instance as intended
- the test instance reported an empty `version` field because the repository `VERSION` file was empty at build time

### Rollback

```bash
cd /opt/new-api-test
docker-compose down
```

## Related
- `docs/solutions/best-practices/header-based-cc-routing-with-single-key-2026-04-22.md`
- `docs/solutions/best-practices/public-group-based-billing-split-requires-code-changes-2026-04-22.md`
