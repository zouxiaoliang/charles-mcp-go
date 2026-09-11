# Contributing / 参与贡献

Thanks for helping make Charles traffic easier to inspect from MCP clients. Installation reports, reproducible examples, documentation fixes and focused code changes are all useful.

欢迎贡献安装反馈、可复现示例、文档修正和功能改进。

## Report a problem

Use the [issue templates](https://github.com/zouxiaoliang/charles-mcp-go/issues/new/choose). Include your OS/architecture, `charles-mcp --version`, client and Charles version, the tool called, the exact error and a minimal reproduction. Replace tokens, cookies, passwords and personal traffic with synthetic values before attaching logs or recordings. English and Chinese reports are welcome.

请附系统、架构、程序版本、客户端、Charles 版本、调用工具及复现步骤。上传日志或会话前替换真实凭据和个人流量，支持中文反馈。

## Work on a change

1. Choose a [good first issue](https://github.com/zouxiaoliang/charles-mcp-go/issues?q=is%3Aissue%20is%3Aopen%20label%3A%22good%20first%20issue%22), or open an issue explaining a larger proposal.
2. Make a focused change. Keep README installation instructions aligned in English and Chinese.
3. Describe the behavior changed and how you verified it in the pull request.

Go 1.26+ is required. From the repository root:

```sh
go build -o bin/charles-mcp ./cmd/charles-mcp
go test -race ./...
go vet ./...
```

Format changed Go files with `gofmt`. Use [the demo](examples/README.md) to verify examples. CI runs on Linux, macOS and Windows; fixes to paths, network errors or timing should account for all three. Most tests use local fixtures and do not need Charles. The opt-in live smoke test uses `CHARLES_MCP_LIVE_SMOKE=1`.

## Where to start

| Area | Location |
|---|---|
| CLI and version reporting | `cmd/charles-mcp/` |
| MCP schemas and application routing | `internal/app/` |
| Import, query, replay and storage | `internal/core/` |
| Reproducible sample and demo | `examples/` |
| Client setup and tutorials | `docs/clients.md`, `docs/tutorials/` |

The domain terminology is documented in [CONTEXT.md](CONTEXT.md), and interface decisions in [docs/adr](docs/adr). Contributions are licensed under the project's [MIT license](LICENSE).
