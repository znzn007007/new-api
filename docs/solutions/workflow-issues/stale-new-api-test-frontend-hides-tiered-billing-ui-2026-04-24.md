---
title: Stale new-api test frontend can hide the tiered billing UI even when the code supports it
date: 2026-04-24
category: workflow-issues
module: new-api test deployment
problem_type: workflow_issue
component: tooling
severity: medium
applies_when:
  - The repository code clearly contains a UI option but the running test environment does not show it
  - A parallel new-api test instance is deployed on sub2api-prod and may be using an older self-built image
  - You need to verify whether the issue is code absence or stale frontend assets in the deployed container
tags: [new-api, sub2api-prod, test-instance, stale-frontend, tiered-billing, tiered-expr, docker, deploy]
symptoms:
  - The admin pricing UI only shows 按量计费 and 按次计费, but not 表达式/阶梯计费
  - Repository code already contains the tiered_expr billing mode radio option
  - The test environment appears healthy, which can mislead you into thinking the feature is disabled by config
root_cause: incomplete_setup
resolution_type: workflow_improvement
---

# Stale new-api test frontend can hide the tiered billing UI even when the code supports it

## Context
The repository already contained the `tiered_expr` pricing mode in the admin UI, but the running `new-api-test` environment on `sub2api-prod` only showed two options: `按量计费` and `按次计费`. That created a false impression that the feature required a hidden config switch.

Code inspection showed the frontend renders the third option directly in `web/src/pages/Setting/Ratio/components/ModelPricingEditor.jsx`, so the likely gap was not feature gating but deployment drift between local source and the test container image.

## Guidance
When a UI option exists in code but is missing from the running test environment, treat it as a **deployment freshness problem first**, not a feature-flag problem.

Use this sequence:

1. **Verify the option exists in code**
   - Check the exact component that should render the control.
   - In this case, `ModelPricingEditor.jsx` hardcodes:
     - `per-token`
     - `per-request`
     - `tiered_expr`

2. **Assume the test instance may be stale**
   - `new-api-test` on `sub2api-prod` is a parallel container, not the production container.
   - It can stay healthy while still serving an older frontend build.

3. **Build a fresh image from the latest repo state**
   - Update the repo to latest `origin/main`
   - Build the image from a clean git snapshot, not the dirty working tree
   - Tag it with a timestamp + commit SHA so it is easy to audit

4. **Update only the test instance**
   - Keep production `new-api` untouched
   - Load the new image on `sub2api-prod`
   - Update `/opt/new-api-test/docker-compose.yml`
   - Restart only `new-api-test`

5. **Re-check the UI after deploy**
   - If the option appears, the issue was stale frontend assets
   - Do not add fake config toggles or patch the code path unnecessarily

For this incident, the deployed fix was:
- build local image from latest `origin/main`
- tag: `new-api-local:test-20260424-112937-6ff9bc4`
- update `/opt/new-api-test/docker-compose.yml`
- restart only `new-api-test`

## Why This Matters
This failure mode is subtle because the container can look fully healthy:
- `/api/status` returns success
- `/v1/models` still returns the expected `401`
- logs look normal

But the frontend bundle can still be old, which means:
- newly added admin controls do not appear
- debugging gets misdirected toward imaginary runtime flags
- engineers may waste time looking for backend switches that do not exist

The right mental model is:

> **Healthy container does not imply fresh frontend assets.**

For self-built `new-api` images, especially parallel test instances, missing UI features often mean the image was built from old source or the test compose file still points to an old tag.

## When to Apply
- A repo grep proves the UI control exists but the running page does not show it
- The environment is a long-lived test container with a self-built image tag
- The issue is isolated to test/staging and production parity is uncertain
- There is no code evidence of a runtime feature flag controlling the missing option

## Examples
### Code evidence that the option is not feature-gated

`web/src/pages/Setting/Ratio/components/ModelPricingEditor.jsx`

```jsx
<Radio value='per-token'>{t('按量计费')}</Radio>
<Radio value='per-request'>{t('按次计费')}</Radio>
<Radio value='tiered_expr'>{t('表达式/阶梯计费')}</Radio>
```

If the running UI does not show the third button, the deployment is stale or serving old assets.

### Deployment evidence on sub2api-prod before refresh

```text
new-api-test     new-api-local:test-20260424-051402-96d99d14
```

This showed the test instance was still on an older self-built image.

### Fresh-image rollout pattern used here

```bash
git pull --ff-only origin main
docker build -t new-api:test-20260424-112937-6ff9bc4 .
docker tag new-api:test-20260424-112937-6ff9bc4 new-api-local:test-20260424-112937-6ff9bc4
docker save -o new-api-local-test-20260424-112937-6ff9bc4.tar new-api-local:test-20260424-112937-6ff9bc4
gcloud compute scp new-api-local-test-20260424-112937-6ff9bc4.tar zkl@sub2api-prod:/tmp/ --zone us-west1-b --project stalwart-elixir-490811-q6
```

Server-side update:

```bash
cd /opt/new-api-test
sudo docker load -i /tmp/new-api-local-test-20260424-112937-6ff9bc4.tar
sudo sed -i 's#^\([[:space:]]*image:[[:space:]]*\).*#\1new-api-local:test-20260424-112937-6ff9bc4#' docker-compose.yml
sudo docker compose up -d
```

Verification:

```bash
curl -sS http://127.0.0.1:3001/api/status
curl -i http://127.0.0.1:3001/v1/models
sudo docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}'
```

Observed result after rollout:
- `new-api-test` switched to `new-api-local:test-20260424-112937-6ff9bc4`
- health check still passed
- the tiered billing UI became available in the test environment

## Related
- `docs/solutions/workflow-issues/sub2api-prod-new-api-test-instance-deploy-2026-04-24.md`
- `C:\Users\zkl\OneDrive\Obsdian\Obsidian\04_Projects\AI出海\new-api 测试实例部署与验证（GCP sub2api-prod）.md`
- `C:\Users\zkl\OneDrive\Obsdian\Obsidian\04_Projects\AI出海\sub2api-prod（GCP）服务器环境与排障速查.md`
