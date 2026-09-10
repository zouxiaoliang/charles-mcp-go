# Charles 抓包与分析

本项目让 AI 客户端通过 MCP 使用 Charles 的流量记录、控制与分析能力。

## Language

**Charles**：
拦截和记录 HTTP、HTTPS 流量的桌面代理工具，是本项目连接的外部应用。
_Avoid_：Go 服务、MCP 服务

**抓包记录（Entry）**：
一次被记录的 HTTP 请求及其相关响应、时间和连接信息；尚未收到响应的请求也属于抓包记录。
_Avoid_：会话、数据包

**捕获（Capture）**：
一组来自实时抓包或会话导入的抓包记录，是查询与分析的共同数据范围。
_Avoid_：单条记录、实时会话

**实时会话（Live Session）**：
一次持续观察 Charles 流量的过程，带有独立的读取进度；结束观察后，其捕获仍可作为历史数据使用。
_Avoid_：Charles 会话、捕获

**Charles 会话（Charles Session）**：
Charles 中的一组抓包记录，也可保存为会话文件。
_Avoid_：MCP 会话、登录会话

**会话导入（Session Import）**：
将已有 Charles 会话文件中的抓包记录纳入可查询和分析的数据集合。
_Avoid_：实时抓包、请求重放

**请求重放（Request Replay）**：
依据已有抓包记录重新向目标服务发送请求，可保留或修改原请求内容。
_Avoid_：会话导入、离线模拟
