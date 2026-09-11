# Three workflows / 三个使用场景

Start with [client setup](../clients.md). The first two workflows use [synthetic session data](../../examples/session.json) and work without Charles. The third uses a temporary local API for a real replay.

先完成客户端配置。前两个教程使用示例会话，无需 Charles；第三个教程通过临时本地 API 验证真实重放。

1. [Find an API error / 定位接口错误](01-debug-api-errors.md)
2. [Map a recorded session / 整理会话调用流程](02-explore-session.md)
3. [Change a request and compare / 修改参数并对比响应](03-replay-and-compare.md)

Replace example file paths with absolute paths on your machine. Use the `capture_id` and `entry_id` returned by each call; IDs in prose are placeholders.

## Multipart inspection / Multipart 检查

- [Inspect a multipart upload / 检查 multipart 上传](multipart-request.md)
