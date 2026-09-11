# Find an API error / 定位接口错误

**Goal:** distinguish a bad login from a subsequent request that lacks authentication. Requires an MCP client and the [sample session](../../examples/session.json); Charles is not required.

**目标：** 找到失败请求，区分登录凭据错误和缺少认证信息。使用示例会话，无需启动 Charles。

## Ask your client

> Import `/absolute/path/to/charles-mcp-go/examples/session.json`. Show only failed requests, quote each response error and explain what to inspect next. Do not send replay requests.

> 导入 `/你的绝对路径/charles-mcp-go/examples/session.json`，只展示失败请求，引用响应错误信息，解释下一步应检查什么，先不重放。

## Tool calls

Call `reverse_import_session`:

```json
{"path":"/absolute/path/to/charles-mcp-go/examples/session.json","source_format":"json"}
```

Use its returned `capture_id` in `reverse_query_entries`:

```json
{"capture_id":"<returned capture_id>","query":{"preset":"errors_only"},"limit":20}
```

For each returned `entry_id`, call `reverse_decode_entry_body`:

```json
{"entry_id":"<returned entry_id>","side":"response"}
```

## Expected evidence

| Request | Status | Response error | Next check |
|---|---|---|---|
| `POST /api/login` | 401 | `invalid_credentials` | Inspect the submitted login fields. |
| `GET /api/orders` | 401 | `missing_demo_token` | Inspect whether the request carries the expected authentication. |

The successful products request is filtered out. Both failures are 401s, but the response bodies distinguish the cases. An AI explanation should cite these messages rather than assume all 401s share one cause.

成功的商品请求不会出现在结果中。两个请求都是 401，但响应内容不同，应引用具体证据判断下一步。

For a real capture, run `charles_status`, `start_live_capture`, reproduce the problem, then `read_live_capture`. Inspect the resulting entries and call `stop_live_capture` when finished. [Next: map the session](02-explore-session.md).
