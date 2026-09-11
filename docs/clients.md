# Connect Claude Desktop or Cursor / 客户端接入

[English README](../README.md) · [中文说明](../README.zh-CN.md)

Install a [release binary](https://github.com/zouxiaoliang/charles-mcp-go/releases/latest) or use `go install`. Run the executable with `--version` first. Use an absolute path in your configuration; GUI applications may not inherit your shell's `PATH`.

先安装程序并运行 `--version`。客户端配置中的 `command` 必须替换为本机实际的绝对路径，图形客户端可能无法读取终端的 `PATH`。

## Claude Desktop

Open **Settings → Developer → Edit Config**, or edit `claude_desktop_config.json` directly:

| System | Configuration file |
|---|---|
| macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Windows | `%AppData%\Claude\claude_desktop_config.json` |

Merge the `charles` entry from [claude-desktop.json](../examples/clients/claude-desktop.json) into your existing `mcpServers` object, replace the placeholders, save and fully restart Claude Desktop.

在设置中打开配置文件，将示例中的 `charles` 合并进已有的 `mcpServers`，替换路径和认证信息，保存后完全退出并重新启动 Claude Desktop。

## Cursor

Use `~/.cursor/mcp.json` for your user configuration. Project configuration can also live at `.cursor/mcp.json`; keep actual credentials in your user configuration.

Merge [cursor.json](../examples/clients/cursor.json), replace the placeholders, then reload Cursor and check its MCP settings for the `charles` server. Tools are used from an agent conversation.

推荐修改用户级 `~/.cursor/mcp.json`。合并示例、替换路径和认证信息后，重新加载 Cursor，在 MCP 设置中确认 `charles` 可用，然后在 Agent 对话中使用。

## Shared configuration

Both clients accept this stdio configuration:

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

Windows example: replace `command` with `"C:\\tools\\charles-mcp.exe"`; the doubled backslashes are required in JSON. [Download a Windows configuration example](../examples/clients/windows.json).

If installed with Go, locate the file using `go env GOBIN` and `go env GOPATH`. With an unset `GOBIN`, it is in `GOPATH/bin` (`charles-mcp.exe` on Windows).

## First successful tool call

**Offline, no Charles needed:** download or clone the repository, then ask the client:

> Import `/absolute/path/to/charles-mcp-go/examples/session.json`. Find requests with errors and quote the response messages. Do not replay them.

> 导入 `/你的绝对路径/charles-mcp-go/examples/session.json`，找出错误请求并展示响应中的错误信息，先不重放。

Expected: three requests in the capture, with two HTTP 401 responses. See the [first tutorial](tutorials/01-debug-api-errors.md).

**Live capture:** enable Charles's Web Interface, match its credentials and proxy port to your configuration, then ask the client to call `charles_status`. You can also run `charles-mcp --check` with the same environment variables. A successful check reports `connected: true`.

实时抓包需要先开启 Charles 的 Web Interface，确保认证和代理端口一致。客户端调用 `charles_status` 返回 `connected: true` 后再开始抓包。HTTPS 证书配置仍在 Charles 中完成。

## Troubleshooting

| Symptom | What to check |
|---|---|
| Server executable not found | Use its absolute path; verify it exists and matches your OS/CPU. |
| Permission denied on macOS/Linux | Run `chmod +x` on the downloaded file. If the OS blocks it, review its normal security prompt. |
| `--version` works, `--check` fails | Check Charles Web Interface, proxy port and credentials. Offline import can still work. |
| Empty tool list | Validate the JSON, merge under `mcpServers`, then restart/reload the client. The server uses stdio, not an HTTP MCP URL. |
| Program waits in the terminal | Normal without flags: it is waiting for MCP messages. Use `--version` to test the executable. |
| Native session import fails | Try Charles XML/JSON export, or configure `CHARLES_CLI_PATH` for conversion. |

Need help? [Open a bug report](https://github.com/zouxiaoliang/charles-mcp-go/issues/new/choose) with your OS, architecture, client, version and redacted error output.

Configuration references: [MCP local server guide](https://modelcontextprotocol.org/docs/2026-07-28/develop/connect-local-servers), [Cursor MCP documentation](https://cursor.com/docs/mcp). Checked September 2026.
