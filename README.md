# Charles MCP Go

使用 Go 重新实现 [heizaheiza/Charles-mcp](https://github.com/heizaheiza/Charles-mcp) 的全部 31 项公开工具能力。通过 stdio 连接 AI 客户端，支持 macOS、Windows、Linux。SQLite、MCP 和解码器均编入可执行文件，无 Python、Node.js 或 CGO 运行依赖。

实时功能连接已有 Charles，**不替代 Charles 代理本身**。离线 XML、JSON 和受支持的原生 ZIP 会话可以直接导入；其他原生格式需要 Charles CLI 转换。

## 构建与运行

需要 Go 1.26 或更新版本；项目固定推荐工具链 1.26.8。

```sh
go build -o bin/charles-mcp ./cmd/charles-mcp
./bin/charles-mcp --version
./bin/charles-mcp --check
./bin/charles-mcp
```

Windows：

```powershell
go build -o bin/charles-mcp.exe ./cmd/charles-mcp
.\bin\charles-mcp.exe --check
```

最后一条命令启动 stdio MCP 服务，等待客户端 JSON-RPC 消息。日志写入 stderr，stdout 只承载协议消息。

Charles 中开启 **Proxy → Web Interface Settings**，设置认证，并确认代理端口。默认连接 `http://control.charles`，经 `http://127.0.0.1:8888` 代理访问，默认认证沿用上游的 `admin` / `123456`。通过环境变量或配置文件改为你实际使用的值。HTTPS 抓包及证书信任仍在 Charles 中配置。

## 客户端配置

支持 stdio MCP 的客户端可使用以下配置，替换为本机可执行文件的绝对路径：

```json
{
  "mcpServers": {
    "charles": {
      "command": "/absolute/path/to/charles-mcp",
      "env": {
        "CHARLES_PROXY_URL": "http://127.0.0.1:8888",
        "CHARLES_USER": "admin",
        "CHARLES_PASS": "your-web-interface-password"
      }
    }
  }
}
```

Windows 的 command 示例：`C:\\tools\\charles-mcp.exe`。HTTP MCP 服务不属于本版接入方式。

## 常用流程

### 实时抓包

1. `charles_status` 检查连接。
2. `start_live_capture {}` 返回 `live_session_id` 和 `capture_id`。
3. `read_live_capture {"live_session_id":"live_…","limit":20}` 读取一页；`has_more=true` 时继续读取。
4. `get_traffic_entry_detail {"entry_id":"…"}` 展开单条记录。
5. `stop_live_capture {"live_session_id":"live_…"}` 保存最后快照并结束实时会话。

`peek_live_capture` 不推进游标。请求完成后的响应更新会沿用同一个 `entry_id`，作为更新再次返回。过滤掉的记录会被消费；需要换条件重新查看时使用 `query_live_capture_entries` 或持久化查询。Charles 清空会话后，新记录不会与旧记录混淆。

默认不清空 Charles；只有 `reset_session=true` 才清空。若服务开启了原本停止的录制，会在最后一个实时会话停止、过期或进程正常退出时恢复；原本就在录制的 Charles 保持原状态。默认会话空闲超时 900 秒，清理周期 30 秒。

### 导入、过滤与分析

```json
{"path":"/absolute/path/session.chls","source_format":"auto"}
```

将以上参数传给 `reverse_import_session`，获取 `capture_id`，然后调用 `reverse_query_entries`：

```json
{
  "capture_id": "cap_…",
  "query": {
    "preset": "api_focus",
    "host_contains": "example.com",
    "method_in": ["POST"],
    "request_json_query": "user.name == 'alice'"
  },
  "limit": 20,
  "offset": 0
}
```

支持路由、方法、状态、错误、资源类型、优先级、Header、Content-Type、解压后的请求/响应文本、JMESPath、大小及时间窗口过滤。查询默认包含所有流量；可选择 `api_focus`、`errors_only`、`page_bootstrap` 或 `all_http`。

`group_capture_analysis` 和 `get_capture_analysis_stats` 提供分组、错误数、状态/资源类型分布、正文大小和时延统计。`group_by` 支持 `host`、`path`、`route`、`method`、`status`、`resource_class`、`content_type`。

### 解码与重放

`reverse_decode_entry_body` 参数包括 `entry_id`、`side=request|response`。支持 gzip、Brotli、zstd、deflate、字符集、JSON、表单和 multipart。Protobuf 另需二进制 `FileDescriptorSet` 文件的 `descriptor_path` 和完整 `message_type`，例如 `example.LoginRequest`。

`reverse_replay_entry` 会向原始目标发送真实请求，保留认证与 Cookie：

```json
{
  "entry_id": "…",
  "query_overrides": {"debug": null, "page": 2},
  "header_overrides": {"X-Test": "yes"},
  "json_overrides": {"/user/name": "bob"},
  "follow_redirects": false,
  "use_proxy": false
}
```

`null` 删除字段，查询和表单支持多值数组。JSON 嵌套字段使用 JSON Pointer（`/a/b`）；普通键名只修改顶层字段。一次只能选择 JSON、表单或文本一种正文修改。默认跟随重定向，默认直接连接目标，不读取环境代理；`use_proxy=true` 才经过 Charles。HTTP 401 等属于已收到响应，区别于网络执行失败。两种结果均保存实验和差异记录。

### 登录、API 与签名分析

`reverse_analyze_live_login_flow`、`reverse_analyze_live_api_flow`、`reverse_analyze_live_signature_flow` 接受 `live_session_id` 或 `capture_id`，返回候选评分、选中证据、解码结果、变异计划和下一步建议。默认不重放；显式 `run_replay=true` 才发送一次选中请求，变异参数放在 `replay` 对象中。

签名发现比较变化字段并做启发式排名，不能证明签名算法。变异计划只是可执行的候选实验，生成计划不会自动发送请求。

## 配置与数据

`--config config.json` 读取 [配置示例](config.example.json)。优先级：命令行参数 → 环境变量 → JSON 文件 → 默认值。

| 环境变量 | 用途 |
|---|---|
| `CHARLES_MCP_DATA_DIR` | SQLite 数据目录 |
| `CHARLES_RECORDINGS_DIR` | `list_recordings` 查找会话文件的目录 |
| `CHARLES_BASE_URL` | Charles 控制地址 |
| `CHARLES_PROXY_URL` | Charles 代理地址；空串表示直连控制地址 |
| `CHARLES_USER` / `CHARLES_PASS` | Web Interface 认证 |
| `CHARLES_CLI_PATH` | Charles 转换命令路径 |

默认数据目录：macOS `~/Library/Application Support/charles-mcp`；Windows `%AppData%\charles-mcp`；Linux `$XDG_CONFIG_HOME/charles-mcp` 或 `~/.config/charles-mcp`。记录与实验跨重启保留；会话空闲超时不删除历史数据。使用 `delete_capture` 显式删除某个捕获及其关联数据。重复导入相同格式、相同内容的文件复用捕获 ID。

详情默认仅展示 2048 字符的正文预览；`max_chars` 可调，`include_raw=true` 返回保存的 base64 正文。原始数据与预览分开，保真等级为 `raw`、`text_only`、`missing`。Header 不做隐式脱敏。单次 Charles 导出、会话文件输入及原生 ZIP 展开总大小上限为 4 GiB（4,294,967,296 字节），解码正文限制 32 MiB。

`reset_environment` 要求明确 action：录制开始/停止、清空会话、退出 Charles、备份/恢复配置。配置操作要求 `config_path`、`backup_path`，可加 `profiles_path`；恢复前先关闭 Charles。普通查询和退出服务不会改写配置。

`--legacy-aliases` 可启用 `filter_func`、`list_sessions`、`proxy_by_time`。工具名称保留，但所有接口以本版 Schema 为准；不是 Python 版的逐参数兼容替换。`--tools` 输出完整 Schema。

## 验证

```sh
go test ./...
go test -race ./...
go vet ./...
bash scripts/build-release.sh
```

测试覆盖格式导入、保真、动态 Protobuf、过滤统计、SQLite 持久化、游标/响应更新/清空、录制所有权、实际 HTTP 重放、分析工作流、MCP Schema 与独立 stdio 进程握手。CI 在三种操作系统运行测试。构建脚本输出三平台 amd64/arm64 二进制。

功能对照与实现依据见 [docs/features.md](docs/features.md)。上游 MIT 许可与来源说明见 [LICENSE](LICENSE)、[PROVENANCE.md](PROVENANCE.md)。
