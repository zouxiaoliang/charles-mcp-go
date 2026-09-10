# 验证记录

验证日期：2026-09-10。本机环境：macOS arm64，Go 1.26.8。

## 已通过

- `go test -race -coverprofile=coverage.out ./...`：三个包全部通过，包括本机 HTTP 重放、实时控制、所有工具族分派、SQLite、导入/解码、工作流和 stdio 子进程握手。
- 语句覆盖率：命令行 60.7%，应用层 73.3%，核心层 74.4%。覆盖率数值不代表未经测试的错误分支也被证明正确。
- `go vet ./...`：通过。
- `scripts/build-release.sh`：生成 macOS、Linux、Windows 的 amd64/arm64 共六个无 CGO 二进制；已用文件格式检查确认架构。
- 本机 `bin/charles-mcp --tools` 与上游 AST 中的 canonical 列表对比：31 项全部存在，新增 delete_capture，无重复，默认共 32 项。
- `bin/charles-mcp --version`：0.1.0。
- `--check`：成功连接本机 Charles，读取到录制状态为开启。
- 真实只读 XML、JSON 导出解析成功。
- 超限原生归档的最小回归先失败再通过：现在直接返回 SizeLimitError，不会进入外部转换并掩盖错误。
- 无遗留调试日志或未实现占位符。

## 验证边界

- 首次联调时真实原生会话导出超过当时的 128 MiB 输入上限，未完成该大型会话的真实导入；后续按用户要求将上限提高为 4 GiB。原生 ZIP、旧格式 CLI 后备和错误处理使用离线夹具验证。
- Windows/Linux 二进制已交叉构建；本机没有这两种操作系统的真实 Charles 联调环境。已提供三平台 CI 测试矩阵，未声称 CI 已在远端执行。
- 所有自动重放测试只发送到测试创建的本机 HTTP 服务；真实 Charles 检查没有重放抓包请求、清空会话或停止用户录制。

## 4 GiB 导出上限更新

- Charles 导出、会话文件输入、原生 ZIP 展开总大小上限统一调整为 4 GiB（4,294,967,296 字节）。
- 边界回归验证：超过旧 128 MiB 上限的归档及恰好 4 GiB 的声明总大小可通过大小检查，超过 4 GiB 返回 SizeLimitError；使用合成 ZIP 元数据，不分配或声称实际导出了 4 GiB 正文。
- `go test ./... -count=1` 与 `go vet ./...` 通过。
- Codex 配置指向的 `bin/charles-mcp` 与六个发布二进制均已重新构建，SHA256SUMS 已更新。已运行的 MCP 进程需重启后加载新上限。
