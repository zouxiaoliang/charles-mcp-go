# Change a request and compare / 修改参数并对比响应

**Goal:** verify a request mutation with a real HTTP response, while preserving the baseline and experiment. The [executable demo](../../examples/README.md) supplies a temporary local API and verifies each step.

**目标：** 修改一项请求参数，通过真实 HTTP 响应验证结果，并保存基线与实验。演示 API 接受示例密码 `demo-valid`；这只是可复现的教学场景。

## Run the demonstrated result

From the repository root:

```sh
go build -o bin/charles-mcp ./cmd/charles-mcp
go run ./examples/demo
```

On Windows, build `bin/charles-mcp.exe` and pass `-server ./bin/charles-mcp.exe`. The demo rewrites the sample destinations to an ephemeral loopback port, records three real responses and imports them into a temporary database.

The final tool call is `reverse_replay_entry`:

```json
{
  "entry_id":"<failed login entry_id returned by the demo>",
  "json_overrides":{"/password":"demo-valid"},
  "use_proxy":false
}
```

The checked result includes:

```json
{"baseline_status":401,"status":200,"status_changed":true,"execution_status":"completed"}
```

An `experiment_id` is also returned. The demo confirms an additional request reached the API, then cleans up its temporary server and database.

程序验证响应从 401 变为 200、产生了实验 ID，并确认本地 API 确实收到了额外请求。演示退出后会清理临时 API 和数据库。

## Use the workflow with your own API

> Show the failing request and explain the proposed mutation. Replay it once against my test API with the changed field, then compare the status and response body with the baseline.

> 展示失败请求和准备修改的字段，在我的测试 API 上重放一次，然后对比原始状态码和响应正文。

Use the entry ID from your actual capture. Replays target the original URL and preserve cookies and authentication; `use_proxy:false` sends directly to that destination. `api.example.test` in the static sample is illustrative, so run the executable demo for a reachable sample endpoint.

Inspect the response preview and `experiment_id`, or query `reverse_list_findings` with `artifact_kind:"experiment"` and the entry ID as `subject_id`. A successful HTTP request can still return a 401 or 500; inspect both `execution_status` and `status`.
