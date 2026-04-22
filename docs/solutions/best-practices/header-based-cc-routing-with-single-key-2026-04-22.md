---
title: Use header-based gateway routing to separate Claude Code and third-party Claude traffic under one external API key
date: 2026-04-22
category: best-practices
module: llm gateway routing
problem_type: best_practice
component: tooling
severity: medium
applies_when:
  - You want one external API key but different internal new-api token groups for Claude Code and third-party Anthropic-compatible clients
  - You do not want to modify new-api source code
  - CC专用 and CC第三方可用 have different cost or compatibility constraints
tags: [new-api, claude-code, anthropic, nginx, gateway-routing, header-based-routing, single-api-key]
---

# Use header-based gateway routing to separate Claude Code and third-party Claude traffic under one external API key

## Context
A common deployment need is to expose a single external API key to customers while keeping separate internal cost buckets and compatibility groups inside new-api. In this repo, group selection is token-driven before channel selection, so the built-in routing path does not natively switch `usingGroup` from request headers alone.

Local code inspection on 2026-04-22 showed:
- `middleware/auth.go` determines the effective group from `token.Group` / user group and writes `ContextKeyUsingGroup`
- `middleware/distributor.go` selects channels after `usingGroup` is already decided
- `service/channel_affinity.go` optimizes channel stickiness inside an already-selected group rather than deciding which group a request should enter

That makes gateway-layer routing the cleanest no-code option when `CC专用` and `CC第三方可用` must remain isolated.

## Guidance
Put a reverse proxy or LLM gateway in front of new-api and let the gateway map one external customer key to different internal new-api tokens based on request headers.

Recommended classification logic for Anthropic-style traffic:
1. **Claude Code dedicated path (CC专用)** only when all of the following are present:
   - `X-Claude-Code-Session-Id`
   - `Anthropic-Version`
   - `User-Agent` contains `claude-cli/`
2. **Default to CC第三方可用** for all other Anthropic-compatible traffic.
3. Keep GPT/OpenAI-family traffic on separate internal tokens or groups as needed.

This preserves a single customer-facing key while still letting internal new-api tokens encode:
- billing group
- model visibility
- retry behavior
- dedicated-vs-third-party upstream separation

Recommended internal design:
- External customer sends one key, for example `Bearer customer_xxx`
- Gateway authenticates that external key once
- Gateway rewrites upstream `Authorization` to one of your internal new-api tokens:
  - `sk_internal_cc_dedicated` -> `CC专用`
  - `sk_internal_cc_3p` -> `CC第三方可用`
- Optional: add an internal observability header such as `X-Route-Class: cc` or `X-Route-Class: cc3p`

Important implementation note:
- Use headers as the routing signal
- Do **not** use new-api channel affinity as the primary group decision mechanism for this requirement
- Treat `User-Agent` as a supporting signal, not the only signal

## Why This Matters
This approach solves the operational problem without code changes:
- customers integrate once with one key
- dedicated Claude Code traffic stays in the expensive or special-purpose bucket
- third-party compatible tools do not accidentally consume dedicated capacity
- `/v1/models` and request behavior stay consistent with the internal token's group and model limits

It also reduces bad UX. If dedicated traffic and third-party traffic share the same internal token group, users can see models they should not use, then hit avoidable upstream failures. Header-based gateway routing avoids that mismatch earlier in the request path.

## When to Apply
- You need one public key but multiple internal billing or compatibility groups
- You already operate Nginx, Caddy, Cloudflare Workers, Kong, APISIX, or another proxy tier
- Claude Code traffic has meaningful differences in cost, headers, or supported upstream behavior
- You want a reversible rollout that avoids touching new-api internals

## Examples
### Recommended decision matrix

| Signal | Claude Code dedicated? | Route |
|---|---:|---|
| `X-Claude-Code-Session-Id` + `Anthropic-Version` + `User-Agent: claude-cli/...` | Yes | `CC专用` |
| `Anthropic-Version` only | No | `CC第三方可用` |
| Anthropic-compatible client with custom UA | No | `CC第三方可用` |
| Missing Claude Code session header | No | `CC第三方可用` |

### Why channel affinity is not enough here

Current repo behavior indicates:
- group selection happens before channel selection
- channel affinity cache keys include `usingGroup`
- affinity helps choose a preferred channel inside the selected group

So affinity is suitable for:
- session stickiness
- cache locality
- repeat selection of the same upstream channel

It is not the right primitive for:
- deciding `CC专用` vs `CC第三方可用`
- replacing gateway-level request classification

### Practical rollout shape
- Keep one customer-facing endpoint and one customer-facing key
- At the gateway, classify Anthropic traffic from headers
- Rewrite to different internal new-api tokens
- Default ambiguous traffic to `CC第三方可用`
- Reserve `CC专用` for confidently identified Claude Code requests

## Related
- Anthropic Claude Code LLM Gateway docs: <https://code.claude.com/docs/en/llm-gateway>
- NGINX map module: <https://nginx.org/en/docs/http/ngx_http_map_module.html>
- NGINX reverse proxy headers: <https://docs.nginx.com/nginx/admin-guide/web-server/reverse-proxy/>
- Kong Route matching (industry-standard host/path/header routing): <https://developer.konghq.com/gateway/entities/route/>
