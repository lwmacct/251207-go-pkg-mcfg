# cfgm

[![License](https://img.shields.io/github/license/lwmacct/251207-go-pkg-cfgm)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/lwmacct/251207-go-pkg-cfgm.svg)](https://pkg.go.dev/github.com/lwmacct/251207-go-pkg-cfgm)
[![Go CI](https://github.com/lwmacct/251207-go-pkg-cfgm/actions/workflows/go-ci.yml/badge.svg)](https://github.com/lwmacct/251207-go-pkg-cfgm/actions/workflows/go-ci.yml)

Schema 驱动的 Go 配置库。一个 `Manager[T]` 同时负责默认值、文件、环境变量、urfave/cli 命令树装配、严格校验和示例配置，避免应用手写 flags 后再靠反射猜测映射关系。

> 当前版本要求 Go 1.27，配置解码基于 `encoding/json/v2`：字段名大小写敏感，拒绝重复 JSON key 和非法 UTF-8，也不会把数字、布尔值等弱转换成字符串。
> `time.Duration` 在所有文本来源中统一使用字符串（例如 `"30s"`）。

## 安装

```bash
go get github.com/lwmacct/251207-go-pkg-cfgm/pkg/cfgm
```

## 运行示例

完整索引和可复制命令见 [`examples/README.md`](examples/README.md)：

| 示例 | 场景 |
| --- | --- |
| [`cli`](examples/cli) | 完整 CLI 应用：自动 flags、优先级、集合、codec、模板和报告 |
| [`config`](examples/config) | 非 CLI 加载、示例配置生成和严格校验 |
| [`custom-source`](examples/custom-source) | 自定义 `Source` 与 `Schema` |

## 定义配置

```go
type Config struct {
    Server ServerConfig `json:"server" desc:"服务端配置"`
}

type ServerConfig struct {
    Addr    string        `json:"addr"    desc:"监听地址"`
    Timeout time.Duration `json:"timeout" desc:"请求超时"`
    Redis   RedisConfig   `json:"redis"   desc:"Redis 配置"`
}

var Manager = cfgm.New(
    DefaultConfig(),
    cfgm.AppName("app"),
    cfgm.CLIAlias("server.addr", "a"),
    cfgm.HideCLI("server.redis.password"),
)
```

配置根类型必须是非指针 struct。`json` tag 是文件、环境变量和 CLI 的稳定 key，`desc` tag 用作 CLI help 和示例配置注释。

### 组合配置

使用 `cfgm:",inline"` 将公共配置类型的字段展开到当前配置层级：

```go
type TLSConfig struct {
    tlsreload.Config `cfgm:",inline"`
    Enabled bool `json:"enabled" desc:"是否启用 TLS"`
}
```

嵌入类型字段的文件、环境变量和 CLI 路径不会增加中间层级。`inline` 必须用于匿名的非指针 struct，该字段不能再声明 `json` tag，也不能为嵌入类型注册 codec；展开后的重复配置 key 会在 `cfgm.New` 时被拒绝。

## 非 CLI 加载

```go
config, err := Manager.Load(ctx,
    cfgm.File("/etc/app/config.yaml", cfgm.Optional()),
    cfgm.Env("APP_"),
)
```

来源按声明顺序合并，后面的来源覆盖前面的来源。除非使用 `WithoutDefaultPaths()`，`Manager` 会先搜索 `DefaultPaths(appName)`；启动阶段也可使用 `MustLoad`，诊断时使用 `LoadReport`。

默认严格拒绝未知字段，并递归校验 struct、struct slice 和 map 中的已知结构。错误路径使用 JSON Pointer（例如 `/routes/0/backends/0`），可直接定位集合中的具体元素。`AllowUnknownKeys()` 只允许额外字段，不会关闭已知字段的类型校验。

## CLI 集成

先构造正常的 urfave 命令树，使用 typed action 接收完整配置：

```go
var serverCommand = &cli.Command{
    Name: "server",
    Action: Manager.Action(func(ctx context.Context, cmd *cli.Command, config *Config) error {
        return runServer(ctx, config)
    }),
}

app := &cli.Command{Name: "app", Commands: []*cli.Command{serverCommand}}
Manager.MustConfigure(app)
```

`MustConfigure` 在 `Run` 前遍历真实命令树，自动添加根 `--config/-c`、`--env-prefix/-e`，并按命令 lineage 修剪同名配置层级。`server.addr` 暴露为 `app server --addr`，`server.redis.url` 暴露为 `--redis.url`；`tools.subcmd.timeout` 自动映射为 `app tools subcmd --timeout`。只有存在 Action 且配置中存在同名 struct 的命令会获得配置 flags。

CLI 加载优先级固定为：

1. 默认值
2. app 对应的默认配置路径
3. 显式 `--config/-c`
4. `--env-prefix/-e`，未设置时使用 app 名推导出的前缀
5. 当前命令中显式设置的 CLI flags

未显式设置的 CLI flag 不参与覆盖。

## 集合值

标量 slice 使用 urfave 的重复 flag：

```bash
app server --tags api --tags edge
```

`[]struct` 和 `[]*struct` 使用 cfgm 的严格 JSON object flag。每次出现添加一个元素，整组替换低优先级来源：

```bash
app server \
  --certificates='{"id":"main","certificate":"op://cert/main","private-key":"op://key/main"}' \
  --certificates='{"id":"api","certificate":"op://cert/api","private-key":"op://key/api"}'
```

使用 `--certificates=[]` 清空集合。`[]` 不能和 object 值混用。object 中的未知字段（包括嵌套 struct slice）会被拒绝。

环境变量中的 slice 和 map 必须是完整 JSON：

```bash
export APP_SERVER_TAGS='["api","edge"]'
export APP_SERVER_CERTIFICATES='[{"id":"main","refresh":"30s"}]'
export APP_LABELS='{"region":"cn"}'
```

这样不会受到逗号分隔规则影响，也能明确表达空数组和空对象。

## 自定义类型

无法由内置 flag 表达的叶子类型使用 `WithCodec`：

```go
manager := cfgm.New(defaults, cfgm.WithCodec(cfgm.Codec[Endpoint]{
    Parse:  ParseEndpoint,
    Format: func(value Endpoint) string { return value.String() },
}))
```

同一 codec 适用于默认值、文件、环境变量和 CLI。`Parse` 与 `Format` 都必填，并且必须可逆：`Parse(Format(value))` 应还原等价值；这样 struct 等自定义叶子类型即使出现在 slice/map 默认值内部，也能安全穿过统一的配置合并管线。

## 模板与文件

默认值和内置 file source 的字符串值默认使用环境变量展开 `${...}`。语法采用 Docker Compose 的只读子集：

| 语法 | 行为 |
| --- | --- |
| `${VAR}` | 使用变量值；未设置时为空字符串 |
| `${VAR:-word}` / `${VAR-word}` | 未设置或为空 / 仅未设置时使用默认值 |
| `${VAR:+word}` / `${VAR+word}` | 已设置且非空 / 已设置时使用替代值 |
| `${VAR:?word}` / `${VAR?word}` | 未设置或为空 / 仅未设置时报错 |
| `$$` | 字面量 `$` |

`word` 支持嵌套展开。`${VAR=word}` 和 `${VAR:=word}` 等赋值语法不受支持，非法或未闭合表达式会返回错误。

文件会先解析为 YAML/JSON，再只展开其中的字符串值；键名和配置结构不会被环境变量改变。数值、布尔值等非字符串字段应直接写入文件，或通过类型化环境变量 source/CLI 提供。

所有来源先按优先级合并，再统一展开最终生效的字符串值。被高优先级来源覆盖的模板不会求值；一次加载中的插值和 `Env(...)` 使用同一份环境快照。

需要保留字面量 `${VAR}` 时写成 `$${VAR}`。`WithoutTemplateExpansion()` 可全局关闭展开。

```go
manager := cfgm.New(defaults, cfgm.WithoutTemplateExpansion())
config, err := manager.Load(ctx, cfgm.File("config.yaml"))
```

## 示例配置

```go
yaml := cfgm.ExampleYAML(DefaultConfig())
jsonBytes := cfgm.MarshalJSON(DefaultConfig())

var files = cfgm.ConfigFiles[Config]{
    Manager:     Manager,
    ExampleFile: "config/config.example.yaml",
    RuntimeFile: "config/config.yaml",
}

func TestWriteConfigExample(t *testing.T)     { files.WriteExample(t) }
func TestRuntimeConfigKeysValid(t *testing.T) { files.ValidateRuntimeConfig(t) }
```

`ValidateRuntimeConfig` 使用 `Manager` 的同一份 Schema 和 codec 规则，不再从 example 文件推导第二套校验语义。

## License

MIT
