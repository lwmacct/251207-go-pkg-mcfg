# go-pkg-config

通用的 Go 配置加载库，支持泛型，可被外部项目复用。

## 特性

- **泛型支持**：适用于任意配置结构体
- **多源合并**：默认值 → 配置文件 → 环境变量 → CLI flags（优先级递增）
- **函数选项模式**：灵活配置，向后兼容
- **环境变量支持**：前缀匹配 + 直接绑定，适合 Docker/K8s 容器化部署
- **自动映射**：CLI flag 名称自动从 `koanf` tag 推导（snake_case → kebab-case）
- **示例生成**：自动根据结构体生成带注释的 YAML 配置示例

## 安装

```bash
go get github.com/lwmacct/251207-go-pkg-config/pkg/config
```

## 快速开始

### 1. 定义配置结构体

```go
// internal/config/config.go
package config

import (
    "time"
    "github.com/lwmacct/251207-go-pkg-config/pkg/config"
)

type Config struct {
    Server ServerConfig `koanf:"server" comment:"服务端配置"`
}

type ServerConfig struct {
    Addr    string        `koanf:"addr" comment:"监听地址"`
    Timeout time.Duration `koanf:"timeout" comment:"超时时间"`
}

func DefaultConfig() Config {
    return Config{
        Server: ServerConfig{
            Addr:    ":8080",
            Timeout: 30 * time.Second,
        },
    }
}

func Load(opts ...config.Option) (*Config, error) {
    return config.Load(DefaultConfig(), opts...)
}
```

### 2. 加载配置

```go
// 仅使用默认值
cfg, err := config.Load(DefaultConfig())

// 使用配置文件
cfg, err := config.Load(DefaultConfig(),
    config.WithConfigPaths(config.DefaultPaths("myapp")...),
)

// 使用环境变量（前缀 MYAPP_）
cfg, err := config.Load(DefaultConfig(),
    config.WithEnvPrefix("MYAPP_"),
)

// 绑定第三方工具环境变量（如 REDIS_URL）
cfg, err := config.Load(DefaultConfig(),
    config.WithEnvBindings(map[string]string{
        "REDIS_URL":         "redis.url",
        "ETCDCTL_ENDPOINTS": "etcd.endpoints",
    }),
)

// 完整示例：配置文件 + 环境变量 + CLI flags
cfg, err := config.Load(DefaultConfig(),
    config.WithConfigPaths("config.yaml", "/etc/myapp/config.yaml"),
    config.WithEnvPrefix("MYAPP_"),
    config.WithEnvBindings(map[string]string{
        "REDIS_URL": "redis.url",
    }),
    config.WithCommand(cmd),
)
```

### 3. 环境变量

#### 前缀匹配（WithEnvPrefix）

| 环境变量 | koanf key |
|---------|-----------|
| `MYAPP_SERVER_ADDR` | `server.addr` |
| `MYAPP_SERVER_TIMEOUT` | `server.timeout` |
| `MYAPP_DEBUG` | `debug` |

转换规则：
1. 移除前缀（如 `MYAPP_`）
2. 转为小写
3. 下划线 `_` 转为点号 `.`

#### 直接绑定（WithEnvBindings）

复用第三方工具的标准环境变量：

```go
config.WithEnvBindings(map[string]string{
    "REDIS_URL":         "redis.url",
    "ETCDCTL_ENDPOINTS": "etcd.endpoints",
    "MYSQL_PWD":         "database.password",
})
```

#### 配置文件绑定（WithEnvBindKey）

在配置文件中定义绑定关系，无需修改代码：

```yaml
# config.yaml
envbind:
  REDIS_URL: redis.url
  ETCDCTL_ENDPOINTS: etcd.endpoints

redis:
  url: "redis://localhost:6379"
```

```go
cfg, err := config.Load(DefaultConfig(),
    config.WithConfigPaths("config.yaml"),
    config.WithEnvBindKey("envbind"),  // 从配置文件读取绑定
)
```

**绑定优先级**：代码中的 `WithEnvBindings` > 配置文件中的 `envbind` 节点。

### 4. 生成配置示例文件

创建测试文件 `internal/config/config_test.go`：

```go
package config

import (
    "testing"
    "github.com/lwmacct/251207-go-pkg-config/pkg/config"
)

func TestGenerateExample(t *testing.T) { config.RunGenerateExampleTest(t, DefaultConfig()) }
func TestConfigKeysValid(t *testing.T) { config.RunConfigKeysValidTest(t) }
```

运行测试生成 `config/config.example.yaml`：

```bash
go test -v -run TestGenerateExample ./internal/config/...
```

生成的示例文件：

```yaml
# 配置示例文件, 复制此文件为 config.yaml 并根据需要修改

# 服务端配置
server:
  addr: ":8080" # 监听地址
  timeout: 30s # 超时时间
```

## API 参考

### config.Load

```go
func Load[T any](defaultConfig T, opts ...Option) (*T, error)
```

加载配置，按优先级合并：
1. `defaultConfig` - 默认值（最低优先级）
2. `WithConfigPaths` - 配置文件（按顺序搜索，找到第一个即停止）
3. `WithEnvPrefix` - 环境变量（前缀匹配）
4. `WithEnvBindKey` / `WithEnvBindings` - 环境变量（绑定，代码优先于配置文件）
5. `WithCommand` - CLI flags（最高优先级，仅当用户明确指定时覆盖）

### config.WithConfigPaths

```go
func WithConfigPaths(paths ...string) Option
```

设置配置文件搜索路径。

### config.WithEnvPrefix

```go
func WithEnvPrefix(prefix string) Option
```

设置环境变量前缀，启用环境变量配置源。

### config.WithEnvBinding / config.WithEnvBindings

```go
func WithEnvBinding(envKey, configPath string) Option
func WithEnvBindings(bindings map[string]string) Option
```

直接绑定环境变量到配置路径，用于复用第三方工具的标准环境变量。

### config.WithEnvBindKey

```go
func WithEnvBindKey(key string) Option
```

设置配置文件中的环境变量绑定节点名称，无需修改代码即可配置映射。

### config.WithCommand

```go
func WithCommand(cmd *cli.Command) Option
```

设置 CLI 命令，启用 CLI flags 配置源。

### config.DefaultPaths

```go
func DefaultPaths(appName ...string) []string
```

返回默认配置文件搜索路径：
- `config.yaml`
- `config/config.yaml`
- `~/.{appName}.yaml`（如果提供 appName）
- `/etc/{appName}/config.yaml`（如果提供 appName）

### config.GenerateExampleYAML

```go
func GenerateExampleYAML[T any](cfg T) []byte
```

根据配置结构体生成带注释的 YAML 示例，通过反射读取 `koanf` 和 `comment` tag。

### config.RunGenerateExampleTest / config.RunConfigKeysValidTest

测试辅助函数，用于在单元测试中生成配置示例和校验配置文件。

## License

MIT
