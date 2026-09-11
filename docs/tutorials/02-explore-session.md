# Map a recorded session / 整理会话调用流程

**Goal:** turn a saved session into an endpoint summary and inspect the login candidate. Use the [sample JSON](../../examples/session.json), or a Charles XML/JSON export. Other native session formats may require the Charles CLI.

**目标：** 将会话文件整理为接口列表，并检查登录候选请求。JSON/XML 可直接导入，其他原生格式可能需要 Charles CLI 转换。

## Ask your client

> Import this recording, group requests by path and summarize their statuses. Inspect the login flow without replaying anything. Separate observed request order from inferred dependencies.

> 导入这份会话，按路径分组并整理状态码，然后分析登录流程，先不要重放。区分实际观察到的请求顺序和推测的依赖关系。

## Tool calls

Import with `reverse_import_session`:

```json
{"path":"/absolute/path/to/charles-mcp-go/examples/session.json","source_format":"auto"}
```

Call `group_capture_analysis` with the returned ID:

```json
{"capture_id":"<returned capture_id>","group_by":"path"}
```

Then call `reverse_query_entries` for request details and ordering:

```json
{"capture_id":"<returned capture_id>","limit":20}
```

Inspect the login candidate with `reverse_analyze_live_login_flow` (the tool also accepts saved captures):

```json
{"capture_id":"<returned capture_id>","decode_bodies":true,"run_replay":false}
```

## Expected evidence

The sample has three paths with one request each. The recorded start times put login first, products second and orders third. The login response reports invalid credentials; the orders response reports a missing demo token. That is evidence to investigate authentication, not proof that request timing caused the failure.

示例中三个接口各出现一次，时间顺序为登录、商品、订单。登录凭据错误和订单缺少 token 提供了认证排查线索；仅凭请求先后不能证明因果关系。

The workflow returns candidate evidence and suggested mutations. No HTTP replay is sent with `run_replay:false`. [Next: replay a change](03-replay-and-compare.md).
