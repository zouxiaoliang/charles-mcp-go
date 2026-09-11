# Try the demo / 运行演示

The [README animation](../docs/assets/demo.gif) is rendered from verified output of the executable demo. It shows real stdio MCP calls and actual HTTP requests to a temporary loopback API. It uses synthetic data and sample credentials; it does not show an AI client's reasoning or a live Charles capture.

README 动画来自演示程序真实执行并校验后的输出：真实 stdio MCP 调用、真实本地 HTTP 请求，使用示例数据和凭据。它不是 AI 客户端推理过程或 Charles 界面录屏。

## Run the complete flow

From the repository root, with Go 1.26+:

```sh
go build -o bin/charles-mcp ./cmd/charles-mcp
go run ./examples/demo -server ./bin/charles-mcp
```

Windows:

```powershell
go build -o bin/charles-mcp.exe ./cmd/charles-mcp
go run ./examples/demo -server ./bin/charles-mcp.exe
```

The demo starts an isolated local HTTP API and a new MCP subprocess with a temporary data directory, makes three baseline requests, imports their results, filters the failed login, decodes its response and replays it with one changed password. It checks the outcome and cleans up the API and temporary data when finished. It does not connect to your running Charles instance.

程序启动临时本地 API 和独立 MCP 进程，发送三条基线请求，导入结果、定位失败登录、解码响应，再修改示例密码重放。预期输出为 `Baseline 401 -> Replay 200`。结果不符合预期时会以非零状态退出，运行结束后清理临时数据。

## Explore the sample in an AI client

Import [session.json](session.json) using its absolute path. The three requests are `POST /api/login` (401), `GET /api/products` (200) and `GET /api/orders` (401). The `api.example.test` host is reserved example data; use the executable demo for a working replay endpoint.

可以直接让 AI 客户端导入 `session.json`，体验离线分析。示例域名不是实际服务；需要重放时运行上面的本地演示。

## Rebuild the animation

```sh
go run ./examples/demo -transcript docs/assets/demo-transcript.json
python3 scripts/render-demo.py
```

The renderer needs Pillow (`python3 -m pip install pillow`), which is optional documentation tooling and not a server runtime dependency. It writes `docs/assets/demo.gif` and a static `demo.png` preview. The animation lasts 32 seconds and reports only the recorded results; it does not invent assistant messages or performance measurements.

See the [three tutorials](../docs/tutorials/README.md) for prompts and tool arguments.
