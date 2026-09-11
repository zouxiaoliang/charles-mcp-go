# Launch copy and directory listing

Ready-to-adapt copy for maintainers. These are drafts, not a record of posts or directory submissions. Use the actual account and the destination community's required format.

供维护者直接调整使用的发布稿与目录资料。下文是草稿，不表示已经在社区发布或提交目录。

## 中文发布稿：V2EX / 掘金

**标题：让 AI 直接分析 Charles 抓包：一个可离线导入、修改参数重放的 Go MCP 服务**

使用 Charles 调试接口时，我希望 AI 客户端能够直接查找请求、读取响应，再用一次受控重放验证修改结果，因此做了 Charles MCP Go。

目前可以通过 stdio MCP 接入 Claude Desktop、Cursor 等客户端：

- 连接已有 Charles，读取实时抓包并按接口、状态码、Header 或正文过滤。
- 导入 XML、JSON 和受支持的原生会话，整理接口列表、分析登录或 API 流程。
- 修改请求中的 JSON 字段、Header 或查询参数，发送真实请求并保存前后对比。
- 单个可执行文件，支持 macOS、Linux、Windows，无需 Python、Node.js 或单独运行数据库。

仓库提供了一个无需 Charles 的示例：导入本地 API 的三条请求，找出登录的 401，解码 `invalid_credentials`，修改示例密码重放后得到 200。每一步使用实际 MCP 调用，演示代码会检查结果。

实时抓包仍需要 Charles 本身及其 Web Interface；离线 XML/JSON 分析可以独立使用。重放会发送真实 HTTP 请求，示例使用临时本地接口和虚构凭据。

- 仓库和演示：https://github.com/zouxiaoliang/charles-mcp-go
- 下载：https://github.com/zouxiaoliang/charles-mcp-go/releases/latest
- 三个教程：https://github.com/zouxiaoliang/charles-mcp-go/tree/main/docs/tutorials

如果你也使用 Charles，欢迎试用并反馈最想自动化的调试步骤。安装问题、客户端兼容情况和具体的使用场景都很有帮助。

## English post: developer communities

**Title: I built a Go MCP server to inspect Charles traffic and verify request changes**

Charles MCP Go lets a stdio MCP client inspect captured HTTP traffic, decode responses, and replay a request with changed headers, query parameters or JSON fields. It runs as one executable on macOS, Linux and Windows and stores captures and experiments locally in SQLite.

You can connect it to an existing Charles instance for live traffic, or import XML/JSON recordings offline. The repository includes Claude Desktop and Cursor configuration examples, three workflow tutorials, and a reproducible demo that finds a 401 login response and verifies a 200 replay against a local fixture API.

The demo uses real MCP tool calls and fake data; it is not an AI client screen recording. Charles is still required for live capture. Replays send real HTTP requests, and signature analysis is heuristic.

Code and demo: https://github.com/zouxiaoliang/charles-mcp-go

Downloads: https://github.com/zouxiaoliang/charles-mcp-go/releases/latest

I would appreciate feedback from Charles users: which part of your traffic-debugging workflow would be most useful to automate? Installation and client compatibility reports are welcome too.

## Short directory description

**Name:** Charles MCP Go

**Description:** Inspect Charles Proxy traffic from MCP clients. Import recordings, decode request/response bodies, analyze API flows, and replay requests with mutations. A single Go executable for macOS, Linux and Windows with local SQLite storage.

**Repository:** https://github.com/zouxiaoliang/charles-mcp-go

**License:** MIT

**Transport:** stdio

**Install:** download a release binary, or `go install github.com/zouxiaoliang/charles-mcp-go/cmd/charles-mcp@latest`

**Configuration:** [client examples](../examples/clients)

**Prerequisites:** Charles with Web Interface enabled for live features. Go 1.26+ only for installation from source. Offline XML/JSON imports do not require Charles.

**Suggested tags:** mcp, model-context-protocol, charles-proxy, go, http-debugging, developer-tools

## Follow up on actual usage

After a post, link it here with its date and platform. Track concrete feedback: installation failures, platform/client combinations, useful workflows and missing examples. Use those reports to choose fixes and future release notes. GitHub traffic and release download counts can show whether visitors are trying the tool; record observations rather than promising a Star target.

发布后记录真实链接、日期和渠道，优先处理用户首次运行遇到的问题。后续更新说明突出具体场景和已解决的问题，避免只罗列提交记录。
