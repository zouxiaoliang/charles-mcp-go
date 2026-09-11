# Decode a gzip response / 解码 gzip 响应

This offline example imports [gzip-response.xml](../../examples/gzip-response.xml), a synthetic Charles XML session. It needs neither Charles nor a reachable API. The XML contains base64-encoded **compressed bytes**, not already-decoded JSON.

本示例使用合成的 Charles XML 会话，全程离线，无需 Charles 或真实接口。XML 中的 base64 保存的是 gzip 压缩后的原始字节，而非已经解压的 JSON。

Ask your MCP client to import this file and decode the response without replaying it. Replace the path below with the file's absolute path on your machine.

让 MCP 客户端导入文件并解码响应，不要重放。请将下面的路径替换为本机绝对路径。

1. `reverse_import_session`:

```json
{"path":"/absolute/path/to/charles-mcp-go/examples/gzip-response.xml","source_format":"xml"}
```

2. `reverse_query_entries`, using the returned `capture_id` / 使用返回的 capture_id：

```json
{"capture_id":"<returned capture_id>","limit":10}
```

3. `reverse_decode_entry_body`, using that entry's `entry_id` / 使用查询返回的 entry_id：

```json
{"entry_id":"<returned entry_id>","side":"response"}
```

## Expected result / 预期结果

There is one `GET https://api.example.test/api/greeting` entry with status 200. Decoding should report `format: "json"`, no warnings, and this value:

查询结果只有一条状态码为 200 的 GET 请求。解码格式应为 `json`，无警告，内容如下：

```json
{"message":"hello from gzip","ok":true}
```

The import removes the XML's base64 transport encoding; the decoder then uses `Content-Encoding: gzip` to decompress the body and `Content-Type: application/json` to parse it. A gzip warning indicates a malformed or already-decompressed fixture and is not the expected successful result.

导入阶段移除 XML 的 base64 外层编码，解码器按 gzip 响应头解压，再按 JSON 类型解析。出现 gzip 警告不属于本例的成功结果。

Run the fixture regression test from the repository root / 在仓库根目录验证示例：

```sh
go test ./internal/core -run TestGzipResponseExample -v
```
