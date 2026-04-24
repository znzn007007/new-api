---
title: Channel test returns 503 "No available accounts" from aigocode upstream
date: 2026-04-20
category: integration-issues
module: controller/channel-test.go + relay/channel/claude + service/error.go
problem_type: integration_issue
component: tooling
symptoms:
  - 点击管理后台“测试渠道/测试按钮”（GET /api/channel/test/:id）返回 message 包含 `bad response status code 503, message: No available accounts...`
  - 后端日志出现 `channel test bad response ... status=503 err=bad response status code 503, message: No available accounts...`
root_cause: config_error
resolution_type: workflow_improvement
severity: medium
tags: [channel-test, anthropic, claude, upstream-503, aigocode]
---

# Channel test returns 503 "No available accounts" from aigocode upstream

## Problem
在 `GET /api/channel/test/:id`（后台“测试渠道”按钮）测试 Anthropic/Claude 渠道时，返回 503，message 显示 `No available accounts`，不清楚是 new-api 自己返回还是上游返回。

## Symptoms
- API 返回（HTTP 200 的 JSON 包装，`success=false`），但 `message` 里包含：
  - `bad response status code 503, message: No available accounts: no available accounts, body: {"error":{...}}`
- 容器日志里会记录：
  - `channel test bad response: ... status=503 err=bad response status code 503, message: No available accounts...`

## What Didn't Work
- 在代码里全文搜索 `No available accounts` 找不到命中：这不是 new-api 内置的错误文案，而是上游 `error.message` 的原文。

## Solution
### 1) 代码层面定位“这段 message 是哪里拼出来的”
`controller/channel-test.go` 的 `testChannel()`：
- 通过 adaptor 发起上游请求：`adaptor.DoRequest(c, info, requestBody)`
- 如果上游 `StatusCode != 200`，调用：
  - `service.RelayErrorHandler(ctx, httpResp, true)`

`service/error.go` 的 `RelayErrorHandler()`：
- `io.ReadAll(resp.Body)` 读取上游响应 body
- 解析通用错误结构 `dto.GeneralErrorResponse`
- 若能解析出 OpenAI-style 的 `{"error":{"message":...}}`，就把该 `error.message` 拼进最终 error 字符串：
  - `bad response status code <code>, message: <error.message>, body: <raw body>`

因此：`No available accounts: no available accounts` 来自上游返回 JSON 的 `error.message` 字段，new-api 只是把它带回并包了一层“bad response status code ...”的诊断信息。

### 2) 抓“完整上游请求”（给上游排查用）
对这类 Claude 渠道（`type=14`）：
- 上游 URL：`<channel.base_url>/v1/messages`（见 `relay/channel/claude/adaptor.go:GetRequestURL()`）
- 关键请求头（见 `relay/channel/claude/adaptor.go:SetupRequestHeader()`）：
  - `x-api-key: <channel key>`
  - `anthropic-version: 2023-06-01`（如果客户端没传，会默认）
  - `content-type: application/json`

“测试按钮”的最小请求体（典型情况下）会是：
```json
{"model":"claude-3-sonnet-20240229","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}
```

在服务器上可以用 `curl -i` 直接复现并拿到上游的 `x-request-id`（强烈建议把这个 request id 一起发给上游）：
```bash
curl -i 'https://api.aigocode.com/v1/messages' \
  -H 'content-type: application/json' \
  -H 'anthropic-version: 2023-06-01' \
  -H 'x-api-key: <YOUR_CHANNEL_KEY>' \
  --data '{"model":"claude-3-sonnet-20240229","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}'
```

示例上游返回（本次现场抓包结果）：
- HTTP：`503`
- 响应头：包含 `x-request-id: <uuid>`
- 响应体：
```json
{"error":{"message":"No available accounts: no available accounts","type":"api_error"},"type":"error"}
```

## Why This Works
new-api 的错误展示并不会“凭空生成” `No available accounts`；它来自上游 `error.message`。
通过直接复现上游请求并获取 `x-request-id`，可以让上游基于他们的日志快速定位为何该 API key/账号池返回“无可用账号”。

## Prevention
- 遇到 `channel test bad response ... status=503` 这类问题，优先把 **上游响应头里的 `x-request-id`**、请求 URL、请求体（脱敏）一起提交给上游。
- 若需要长期诊断：考虑在运维侧增加“按 request id / channel id”可检索的上游错误记录（避免只剩下前端 message）。

## Related Issues
- （暂无）

