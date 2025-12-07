# go-pkg-config

通用的 Go 配置加载库，支持泛型，可被外部项目复用。

## 特性

- **泛型支持**：适用于任意配置结构体
- **多源合并**：默认值 → 配置文件 → CLI flags（优先级递增）
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
    "github.com/urfave/cli/v3"
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

func Load(cmd *cli.Command, configPaths []string) (*Config, error) {
    return config.Load(cmd, configPaths, DefaultConfig())
}
```

### 2. 加载配置

```go
// 使用默认搜索路径
cfg, err := config.Load(cmd, pkgconfig.DefaultPaths("myapp"))

// 自定义搜索路径
cfg, err := config.Load(cmd, []string{"config.yaml", "/etc/myapp/config.yaml"})

// 不搜索配置文件（仅默认值 + CLI flags）
cfg, err := config.Load(cmd, nil)
```

### 3. 生成配置示例文件

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
func Load[T any](cmd *cli.Command, configPaths []string, defaultConfig T) (*T, error)
```

加载配置，按优先级合并：
1. `defaultConfig` - 默认值（最低优先级）
2. `configPaths` - 配置文件（按顺序搜索，找到第一个即停止）
3. `cmd` - CLI flags（最高优先级，仅当用户明确指定时覆盖）

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
