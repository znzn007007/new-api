---
title: Public group based billing split requires code changes in current new-api flow
date: 2026-04-22
last_updated: 2026-04-23
category: best-practices
module: group billing and request routing
problem_type: best_practice
component: tooling
severity: medium
applies_when:
  - You want users to see only a small set of public groups, but billing must split into more internal groups
  - Nginx or another gateway sits in front of new-api and must decide routing before new-api middleware runs
  - The same public group needs different effective billing behavior based on model family or request type
  - You are implementing tag-aware billing resolution after authentication but before final channel selection
tags: [new-api, group-routing, billing, nginx, token-group, internal-groups]
---

# Public group based billing split requires code changes in current new-api flow

## Context
This repo already supports group ratios, user-group-to-using-group special ratios, and special usable groups. That is enough for exposing a small number of public groups to users and for applying different prices across groups.

The gap appears when one public group must represent multiple internal billing buckets. In the discussed deployment, users should continue to see only two public groups:

- `ask-第三方渠道`
- `ask-CC专用`

But the actual billing model is finer-grained:

- GPT traffic
- Claude third-party traffic
- Claude Code traffic

At first glance, this seems solvable with hidden groups plus Nginx pre-routing. After checking the code path, that is not sufficient by itself.

Later implementation and review work added an important second lesson: **even after adding code-level public-group → tag billing resolution, the new layer must preserve old routing fallback behavior and billing consistency.** A tag-aware split that narrows routing too early or lets retry-time billing diverge from pre-consume billing will regress existing traffic.

## Guidance
In the current new-api design, the user-selected group is resolved inside the service after token authentication, not at the gateway boundary.

The important flow is:

1. The token stores the selected group in `token.Group`
2. `middleware/auth.go` loads the token and resolves the effective using-group
3. That resolved group is written into request context as `ContextKeyUsingGroup`
4. `middleware/distributor.go` later reads `ContextKeyUsingGroup` and only then starts channel selection

That means a front gateway such as Nginx cannot naturally know whether the request belongs to `ask-第三方渠道` or `ask-CC专用`, unless you add some request-visible routing signal outside of current new-api behavior.

So under the current constraints, there are only two realistic directions:

1. **Change the external integration contract** so the gateway can distinguish traffic before new-api runs
   - different hostnames
   - different paths
   - explicit routing headers
   - different public keys for different lanes

2. **Change new-api code** so public groups can be rewritten into internal billing groups after token auth but before billing/channel decisions are finalized

If you do not want to change the external client contract, then code changes are the correct path.

But those code changes need tighter guardrails than “resolve one tag, then hard-filter channels by that tag.” The safer mental model is:

1. **Public group remains the user-facing truth**
2. **Tag resolution provides billing attribution and preferred routing context**
3. **Selection must preserve legitimate fallback capacity unless the operator explicitly forces a hard route**
4. **Pre-consume, retry, realtime billing, logs, and pricing display must all share the same effective multiplier decision**

In practice, that means:

- An explicit `public group + model -> tag` override may justify hard selection, because the operator asked for it
- Automatic inference should be treated more cautiously:
  - if it is ambiguous, do not fail a request that used to be routable
  - if there is one tagged pool plus untagged fallback channels, do not make the untagged fallback unreachable unless that behavior is explicitly intended
- Retry-time routing changes must not silently increase final settlement beyond what was admitted at pre-consume time
- A resolved ratio of `0` is a valid business decision (free route), not a missing value

## Why This Matters
Without writing this down, it is easy to spend time trying combinations of:

- hidden groups
- `group_special_usable_group`
- `GroupGroupRatio`
- Nginx pre-routing
- channel affinity

Those tools do not solve this exact problem by themselves.

The core reason is architectural timing:

- **Nginx runs before new-api knows the token-selected group**
- **new-api pricing and channel decisions happen after auth populates the internal using-group context**

Once you move the split inside new-api, the next risk is **architectural coupling**:

- If tag resolution is treated as both attribution and mandatory hard route selection, you can accidentally remove fallback channels that kept the old system healthy
- If settlement uses a fresher effective ratio than pre-consume or admission checks, billing becomes internally inconsistent
- If `0` is treated as “unset,” free routes stop being free in some code paths

So the implementation bar is not just “support public-group + tag rules.” It is:

> **support public-group + tag rules without making routing stricter than intended or letting effective billing drift across code paths**

So the requirement is not just “add more groups.” It is really “support public-group to internal-billing-group remapping at the correct stage of the request lifecycle.”

Recognizing that early prevents dead-end configuration work and makes the design discussion cleaner.

## When to Apply
- A public group is only a user-facing concept and should fan out into multiple hidden billing groups
- Internal billing behavior depends on model family, request type, or client type
- The gateway cannot infer the correct internal lane from path, host, header, or another request-visible signal alone
- You want to keep the current public UX but refine internal accounting
- You are adding tag-aware billing while still needing old fallback channels and retry behavior to remain safe

## Examples
### Example: why pure hidden-group config is not enough
You can create hidden groups such as:

- `ask-gpt-internal`
- `ask-claude-3p-internal`
- `ask-cc-internal`

And you can keep only these public groups visible:

- `ask-第三方渠道`
- `ask-CC专用`

But if the request arrives at Nginx with only a bearer token and a normal API payload, Nginx still does not know which public group was selected in new-api, because that selection lives in `token.Group` and is only resolved after `middleware/auth.go` runs.

### Example: the minimum code-change idea
A small internal remap layer can work like this:

- if public group is `ask-第三方渠道` and model is GPT → use `ask-gpt-internal`
- if public group is `ask-第三方渠道` and model is Claude third-party → use `ask-claude-3p-internal`
- if public group is `ask-CC专用` and request matches Claude Code signals → use `ask-cc-internal`

That keeps the public UX stable while letting billing and channel routing use more precise internal groups.

### Example: what not to do after adding tag-aware resolution
Suppose `ask-第三方渠道 + claude-3-7-sonnet` currently has:

- one tagged pool: `Claude 第三方`
- one untagged fallback pool

If automatic inference sees only one non-empty tag and then hard-filters selection to that tag, the untagged fallback disappears. Traffic will now fail as soon as the tagged pool is degraded, even though the old system could still route successfully.

The safer approach is:

- use `Claude 第三方` as billing attribution and preferred routing signal
- preserve the untagged fallback unless an operator-configured override says this model must only use that tag

### Example: retry-time billing drift
If the first attempt pre-consumes quota using a cheaper effective multiplier, but retry lands on a different public group or tag with a higher multiplier, final settlement can exceed what the request was admitted for.

Avoid this by treating **effective billing resolution as a shared decision artifact** reused by:

- pre-consume checks
- retry path updates
- final settlement
- pricing API output
- log rendering

### Example: free route handling
If a tag-specific ratio is intentionally `0`, realtime and WebSocket paths must preserve that literal `0`. A `0` multiplier means “free,” not “missing, fall back to default group ratio.”

## Related
- `docs/solutions/best-practices/header-based-cc-routing-with-single-key-2026-04-22.md` — documents the earlier no-source-change gateway routing idea and its limits
- `docs/reports/2026-04-23-public-group-billing-split-review-report.md` — records the concrete regressions found when the first tag-aware implementation narrowed routing or billing too aggressively
- Relevant code path:
  - `middleware/auth.go`
  - `middleware/distributor.go`
  - `model/token.go`
  - `service/group_tag_resolver.go`
  - `service/channel_select.go`
  - `relay/helper/price.go`
  - `service/quota.go`
