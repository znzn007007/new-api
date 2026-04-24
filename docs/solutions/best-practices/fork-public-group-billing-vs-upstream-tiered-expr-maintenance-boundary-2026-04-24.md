---
title: Keep fork public-group billing attribution separate from upstream tiered_expr unless expression context is expanded
date: 2026-04-24
last_updated: 2026-04-24
category: best-practices
module: billing and merge maintenance
problem_type: best_practice
component: group billing, tiered billing, merge strategy
severity: high
applies_when:
  - Your fork already implements public-group -> channel-tag -> effective billing attribution logic
  - Upstream adds or evolves tiered_expr dynamic billing
  - You need to decide whether upstream expressions can replace fork-specific billing attribution without source changes
  - You want future upstream merges to preserve routing, billing, and log consistency
tags: [new-api, fork-maintenance, billing, public-group, channel-tag, tiered-expr, merge-boundary]
---

# Keep fork public-group billing attribution separate from upstream tiered_expr unless expression context is expanded

## Context

This fork already has a concrete business rule:

1. users still choose a **public group**
2. the system resolves a unique **channel tag / billing attribution**
3. routing and billing use that resolved attribution
4. frontend pricing, actual charge, admin logs, and user logs must stay on the same explanation path

Upstream now adds `tiered_expr` billing. That is useful, but it solves a different layer of the problem.

Code inspection on 2026-04-24 showed:

- `service/group_tag_resolver.go` is still the place that resolves:
  - `MatchedTag`
  - `BillingAttribution`
  - `BillingRatioSource`
  - `BillingRatioFallback`
- `relay/helper/price.go` still computes `groupRatioInfo` from that resolved attribution path first
- when a model uses `billing_mode == "tiered_expr"`, `modelPriceHelperTiered()` runs **after** `groupRatioInfo.GroupRatio` is already decided
- `pkg/billingexpr` currently exposes token variables, request body/header access, and time helpers to expressions
- `pkg/billingexpr` does **not** currently expose:
  - public group
  - resolved channel tag
  - billing attribution
  - final selected channel
- `service/log_info_generate.go` still records fork-specific attribution fields separately from tiered billing metadata

So the current upstream dynamic billing layer is **additive pricing logic**, not a replacement for fork-specific attribution and routing resolution.

## Guidance

Treat the two systems as different responsibilities:

### 1. Fork public-group logic remains the attribution truth

Use the fork path to decide:

- which public group the request belongs to
- which channel tag the request should be attributed to
- whether the request is using tag-specific billing or fallback billing
- which routing boundary is safe for channel selection

That decision should remain the source of truth for:

- routing scope
- billing attribution
- effective ratio fallback
- admin/user log explanation

### 2. Upstream `tiered_expr` is only a model-side pricing layer

Use `tiered_expr` only for questions like:

- different prices for different token bands
- request-aware surcharges or discounts
- tool / stream / cache / image / audio pricing
- time-based or request-parameter-based billing adjustments

Do **not** assume `tiered_expr` can, by itself and without code changes, answer:

- which tag this request belongs to
- which internal billing bucket should be used
- whether a fallback path should be preserved
- whether pricing attribution should say `GPT`, `Claude 第三方`, or public-group default

### 3. Do not merge away the fork attribution layer just because upstream gains richer pricing

The safe merge rule is:

> upstream `tiered_expr` may refine the final price formula, but it must not replace fork attribution/routing logic unless expressions can read the same attribution context and preserve the same guarantees.

Without that guardrail, a future merge can quietly break:

- unique tag resolution
- fallback behavior
- pre-consume vs settlement consistency
- pricing display vs actual billing consistency
- log explanations

## Why This Matters

The fork's requirement is not only “support dynamic pricing.”

It is:

> **under one public-group UX, resolve one stable internal attribution path and keep routing, billing, and logs aligned**

Upstream `tiered_expr` does not currently provide that full contract. It only provides a richer way to compute the price **after** another part of the system has already chosen the attribution path.

So in the current codebase, these are not competing implementations:

- fork logic = **who should this request bill through**
- upstream `tiered_expr` = **how much should that already-attributed request cost**

Confusing those layers leads to bad merge decisions.

## When to Apply

- A future upstream merge touches:
  - `pkg/billingexpr/*`
  - `setting/billing_setting/*`
  - `relay/helper/price.go`
  - `service/log_info_generate.go`
  - pricing/log usage frontend components
- Someone proposes “just use upstream dynamic billing instead of the fork logic”
- Someone wants no-code convergence from public-group split into expression-only billing
- Tests start failing around pricing, log rendering, or channel-tag attribution after an upstream sync

## Maintenance Rules

### Rule 1: Preserve attribution first, then pricing

When resolving merge conflicts, keep this order:

1. public-group / tag attribution correctness
2. fallback behavior correctness
3. routing boundary correctness
4. final pricing expression correctness
5. frontend/log rendering parity

If an upstream change improves dynamic pricing but weakens attribution or fallback guarantees, keep the fork behavior.

### Rule 2: Do not treat expression capability as attribution capability

Before saying upstream dynamic billing can replace the fork implementation, verify expressions can read all required runtime context.

At minimum, no-code convergence is **not** possible until expressions can safely access context equivalent to:

- `publicGroup`
- `MatchedTag`
- `BillingAttribution`
- possibly final selected channel or selected channel tag

### Rule 3: Keep logs bilingual in meaning, not just in fields

Whenever upstream changes pricing/log rendering:

- admin logs must still show:
  - selected public group
  - matched channel tag
  - effective billing ratio
- user logs must still show:
  - billing attribution
  - billing ratio
  - default-rule fallback when applicable

If upstream tiered logs are added, they should supplement the fork attribution story, not overwrite it.

### Rule 4: Shared billing decision artifacts must stay shared

The same effective attribution decision must continue to feed:

- pre-consume billing
- actual settlement
- frontend model pricing display
- admin log detail
- user log detail

If one path starts using a fresher or different interpretation than another, treat that as a regression.

### Rule 5: No-code convergence is currently a non-goal

Under the current implementation boundary:

- “move real billing into expressions without code changes” is not a supported maintenance target
- the fork implementation remains primary for public-group billing split
- `tiered_expr` is optional and secondary unless future code changes expand expression context

## Review Checklist For Future Merges

When upstream billing changes arrive, check:

- [ ] Is `service/group_tag_resolver.go` still the attribution source of truth?
- [ ] Does `relay/helper/price.go` still preserve fork fallback semantics?
- [ ] Does `tiered_expr` still run after attribution has been resolved?
- [ ] Did any frontend pricing code accidentally stop showing fork attribution-based pricing?
- [ ] Did admin/user logs keep fork attribution fields and fallback wording?
- [ ] Did any new upstream test or refactor silently assume expressions can replace attribution logic?

## Examples

### Safe example

- public-group resolver chooses `Claude 第三方`
- fork ratio path resolves effective ratio
- model also has a `tiered_expr`
- final cost = expression-derived raw cost converted to quota and multiplied by the already-resolved effective ratio

This is safe because attribution and pricing still have separate, explicit owners.

### Unsafe example

- upstream adds a richer expression editor
- someone deletes or bypasses public-group tag resolution
- pricing is now derived only from request params and token counts

This is unsafe because the system no longer knows how to explain:

- why this request belongs to one internal bucket instead of another
- why fallback did or did not happen
- why routing and billing still match

## Recommended Current Position

For this fork today:

- keep the fork's public-group billing split logic as the primary implementation
- accept upstream `tiered_expr` as an optional pricing enhancement layer
- do not attempt to replace attribution/routing logic with expressions without new code support

## Related

- `docs/brainstorms/2026-04-22-public-group-billing-split-requirements.md`
- `docs/plans/2026-04-22-001-feat-public-group-billing-split-plan.md`
- `docs/reports/2026-04-23-public-group-billing-split-review-report.md`
- `docs/solutions/best-practices/public-group-based-billing-split-requires-code-changes-2026-04-22.md`
- `service/group_tag_resolver.go`
- `relay/helper/price.go`
- `pkg/billingexpr/expr.md`
- `service/log_info_generate.go`
