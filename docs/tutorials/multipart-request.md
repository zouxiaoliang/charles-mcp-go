# Inspect a multipart upload / 检查 multipart 上传

Import [multipart-request.json](../../examples/multipart-request.json) to inspect one text field and one tiny fake file. This is synthetic, offline data: no Charles instance, network request or real file upload is needed. The example demonstrates inspection only, not multipart mutation or replay.

导入示例可检查一个文本字段和一个微型虚构文件。数据为离线合成数据，无需 Charles、联网请求或实际上传。本例只演示检查，不涉及 multipart 修改或重放。

Ask your MCP client to import the fixture and decode the **request** body without replaying it. Replace the example path with an absolute path on your machine.

让 MCP 客户端导入文件并解码**请求体**，不要重放。请替换成本机绝对路径。

1. `reverse_import_session`:

```json
{"path":"/absolute/path/to/charles-mcp-go/examples/multipart-request.json","source_format":"json"}
```

2. `reverse_query_entries`, using the returned `capture_id` / 使用返回的 capture_id：

```json
{"capture_id":"<returned capture_id>","limit":10}
```

3. `reverse_decode_entry_body`, using the returned `entry_id` / 使用查询返回的 entry_id：

```json
{"entry_id":"<returned entry_id>","side":"request"}
```

## Expected result / 预期结果

The entry is `POST https://api.example.test/api/upload`, status 200. Decoding reports `format: "multipart"` with two parts in `value`:

应只有一条状态码为 200 的 POST 请求。解码格式为 `multipart`，`value` 中包含两个部分：

| name / 字段名 | filename / 文件名 | byte_length / 字节数 | preview / 内容 |
|---|---|---|---|
| `description` | empty / 空 | 19 | `Offline upload demo` |
| `attachment` | `hello.txt` | 23 | `hello from a fake file` followed by a newline / 末尾带换行 |

The request Content-Type includes `boundary=demo-boundary`; each delimiter in the body matches it and uses CRLF line endings. The final delimiter closes the multipart message. Both part previews should be complete (`truncated: false`) and decoding should produce no warnings. JSON string escapes preserve the CRLF bytes when importing this text-only recording.

请求 Content-Type 中的 boundary 与请求体分隔符一致，分隔行使用 CRLF，最后一个分隔符结束消息。两个部分均应完整显示，无截断、无警告。JSON 转义保证导入时保留 CRLF 字节。

Verify the checked-in fixture from the repository root / 在仓库根目录验证：

```sh
go test ./internal/core -run TestMultipartRequestExample -v
```
