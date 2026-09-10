# 来源

本项目使用 Go 重实现 https://github.com/heizaheiza/Charles-mcp 的功能。

- 源码基线：`eb5d0927739ab278cc8903e55e40160efee15c45`（3.1.0rc1）。
- 上游许可：MIT，版权归属保留在根目录 LICENSE。
- Go 代码与测试为本次重实现编写，参考上游公开工具契约、Charles 格式解析、签名启发式和工作流。
- 数据模型、MCP 参数、增量同步与显式环境控制经过重新设计；不承诺 Python 接口逐参数兼容。
- 第三方 Go 依赖及精确版本记录于 go.mod/go.sum，各自适用其模块许可。
