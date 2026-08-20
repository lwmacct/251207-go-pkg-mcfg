# cfgm examples

所有命令均从仓库根目录执行。示例按真实使用场景组织，每个目录可以独立运行和阅读。

| 示例 | 场景 | 覆盖能力 |
| --- | --- | --- |
| [`cli`](cli) | CLI 服务应用 | 自动 flags、来源优先级、`LoadReport`、集合、codec、模板 |
| [`config`](config) | 非 CLI 配置生命周期 | 默认值、文件、环境变量、示例生成、严格校验 |
| [`custom-source`](custom-source) | 扩展配置来源 | 实现 `Source`、读取 `Schema`、来源报告 |

## CLI 应用

[`cli`](cli) 是主要示例。一条命令同时展示默认值、文件、环境变量和 CLI 的覆盖顺序：

```bash
REDIS_URL=redis://localhost:6379/2 \
REDISCLI_AUTH=secret \
CFGM_EXAMPLE_SERVER_ADDR=:9000 \
  go run ./examples/cli \
  --config examples/cli/config.yaml \
  server \
  --addr=:10000 \
  --timeout=10s \
  --tags=api --tags=edge \
  --upstream=svc://cli \
  --certificates='{"id":"main","certificate":"file:///main.crt","private-key":"file:///main.key"}'
```

最终 `addr` 来自 CLI；输出还会列出每个配置来源及其贡献的 key。示例同时体现：

- `CLIAlias("server.addr", "a")` 添加 `--addr/-a`；
- `HideCLI("server.redis.password")` 让密码只能来自文件或环境变量；
- 标量 slice 使用重复 flag，`[]struct` 使用 JSON object flag；
- `--certificates=[]` 可以显式清空集合；
- `upstream` 是 struct，通过 codec 在所有来源中表现为 `svc://host` 字符串；
- 配置文件中的 `${VAR:-fallback}` 在所有来源合并后展开。

环境变量中的集合使用完整 JSON：

```bash
CFGM_EXAMPLE_SERVER_TAGS='["env","json"]' \
  go run ./examples/cli server
```

## 配置文件生命周期

[`config`](config) 展示不使用 CLI 时的常规加载：

```bash
APP_NAME=from-env SERVICE_TOKEN=secret \
  go run ./examples/config
```

优先级为默认值 → [`config.yaml`](config/config.yaml) → `APP_` 环境变量。配置文件中的服务地址和令牌支持 `${...}` 模板。

生成 [`config.example.yaml`](config/config.example.yaml)、校验运行配置，并验证严格/宽松未知字段策略：

```bash
go test -v ./examples/config
```

生成与校验使用同一个 `Manager`，不会维护第二套 Schema。

## 自定义来源

[`custom-source`](custom-source) 实现内存 `Source`，使用 `Schema.Has` 检查目标配置，并通过 `LoadReport` 暴露来源信息：

```bash
go run ./examples/custom-source
```
