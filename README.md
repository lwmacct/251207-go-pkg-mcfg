# go-pkg-config

[![License](https://img.shields.io/github/license/lwmacct/251207-go-pkg-cfgm)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/lwmacct/251207-go-pkg-cfgm.svg)](https://pkg.go.dev/github.com/lwmacct/251207-go-pkg-cfgm)
[![Go CI](https://github.com/lwmacct/251207-go-pkg-cfgm/actions/workflows/go-ci.yml/badge.svg)](https://github.com/lwmacct/251207-go-pkg-cfgm/actions/workflows/go-ci.yml)
[![codecov](https://codecov.io/gh/lwmacct/251207-go-pkg-cfgm/branch/main/graph/badge.svg)](https://codecov.io/gh/lwmacct/251207-go-pkg-cfgm)
[![Go Report Card](https://goreportcard.com/badge/github.com/lwmacct/251207-go-pkg-cfgm)](https://goreportcard.com/report/github.com/lwmacct/251207-go-pkg-cfgm)
[![GitHub Tag](https://img.shields.io/github/v/tag/lwmacct/251207-go-pkg-cfgm?sort=semver)](https://github.com/lwmacct/251207-go-pkg-cfgm/tags)

通用的 Go 配置加载库，支持泛型，可被外部项目复用。

<!--TOC-->

## Table of Contents

- [特性](#特性) `:31+10`
- [安装](#安装) `:41+6`
- [快速开始](#快速开始) `:47+188`
  - [1. 定义配置结构体](#1-定义配置结构体) `:49+34`
  - [2. 加载配置](#2-加载配置) `:83+35`
  - [3. 环境变量](#3-环境变量) `:118+54`
  - [4. 测试驱动的配置管理](#4-测试驱动的配置管理) `:172+63`
- [模板语法](#模板语法) `:235+52`
  - [基本语法](#基本语法) `:243+10`
  - [内置函数](#内置函数) `:253+21`
  - [使用示例](#使用示例) `:274+13`
- [License](#license) `:287+3`

<!--TOC-->

## 特性

- **泛型支持**：适用于任意配置结构体
- **多源合并**：默认值 → 配置文件 → 环境变量 → CLI flags（优先级递增）
- **函数选项模式**：灵活配置，向后兼容
- **环境变量支持**：前缀匹配 + 直接绑定，适合 Docker/K8s 容器化部署
- **自动映射**：CLI flag 名称自动从 `koanf` tag 推导（snake_case → kebab-case）
- **示例生成**：自动根据结构体生成带注释的 YAML 配置示例
- **模板展开**：支持环境变量引用、默认值和多级 fallback

## 安装

```bash
go get github.com/lwmacct/251207-go-pkg-cfgm/pkg/cfgm
```

## 快速开始

### 1. 定义配置结构体

```go
// internal/config/config.go
package cfgm

import (
    "time"
    "github.com/lwmacct/251207-go-pkg-cfgm/pkg/cfgm"
)

type Config struct {
    Server ServerConfig `koanf:"server" desc:"服务端配置"`
}

type ServerConfig struct {
    Addr    string        `koanf:"addr" desc:"监听地址"`
    Timeout time.Duration `koanf:"timeout" desc:"超时时间"`
}

func DefaultConfig() Config {
    return Config{
        Server: ServerConfig{
            Addr:    ":8080",
            Timeout: 30 * time.Second,
        },
    }
}

func Load(opts ...cfgm.Option) (*Config, error) {
    return cfgm.Load(DefaultConfig(), opts...)
}
```

### 2. 加载配置

```go
// 使用默认值 + 默认配置文件路径 (config.yaml, config/config.yaml)
cfg, err := cfgm.Load(DefaultConfig())

// 使用应用专属配置文件路径 (.myapp.yaml, ~/.myapp.yaml, /etc/myapp/config.yaml 等)
cfg, err := cfgm.Load(DefaultConfig(),
    cfgm.WithConfigPaths(cfgm.DefaultPaths("myapp")...),
)

// 使用环境变量（前缀 MYAPP_）
cfg, err := cfgm.Load(DefaultConfig(),
    cfgm.WithEnvPrefix("MYAPP_"),
)

// 绑定第三方工具环境变量（如 REDIS_URL）
cfg, err := cfgm.Load(DefaultConfig(),
    cfgm.WithEnvBindings(map[string]string{
        "REDIS_URL":         "redis.url",
        "ETCDCTL_ENDPOINTS": "etcd.endpoints",
    }),
)

// 完整示例：配置文件 + 环境变量 + CLI flags
cfg, err := cfgm.Load(DefaultConfig(),
    cfgm.WithConfigPaths("config.yaml", "/etc/myapp/config.yaml"),
    cfgm.WithEnvPrefix("MYAPP_"),
    cfgm.WithEnvBindings(map[string]string{
        "REDIS_URL": "redis.url",
    }),
    cfgm.WithCommand(cmd),
)
```

### 3. 环境变量

#### 前缀匹配（WithEnvPrefix）

| 环境变量                     | koanf key              |
| ---------------------------- | ---------------------- |
| `MYAPP_SERVER_ADDR`          | `server.addr`          |
| `MYAPP_SERVER_TIMEOUT`       | `server.timeout`       |
| `MYAPP_DEBUG`                | `debug`                |
| `MYAPP_CLIENT_REV_AUTH_USER` | `client.rev-auth-user` |

转换规则：

1. 移除前缀（如 `MYAPP_`）
2. 转为小写
3. 点号 `.` 和连字符 `-` 都转为下划线 `_`

**注意**：通过反射自动生成所有 koanf key 的绑定，因此支持任意命名的 koanf key（包括包含连字符的 key）。

#### 直接绑定（WithEnvBindings）

复用第三方工具的标准环境变量：

```go
cfgm.WithEnvBindings(map[string]string{
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
cfg, err := cfgm.Load(DefaultConfig(),
    cfgm.WithConfigPaths("config.yaml"),
    cfgm.WithEnvBindKey("envbind"),  // 从配置文件读取绑定
)
```

**绑定优先级**：代码中的 `WithEnvBindings` > 配置文件中的 `envbind` 节点。

### 4. 测试驱动的配置管理

本库提供 `ConfigTestHelper` 测试辅助工具，通过单元测试实现配置示例生成和配置校验。

创建测试文件 `internal/config/config_test.go`：

```go
package cfgm

import (
    "testing"
    "github.com/lwmacct/251207-go-pkg-cfgm/pkg/cfgm"
)

// 定义一次，复用多处
var helper = cfgm.ConfigTestHelper[Config]{
    ExamplePath: "config/config.example.yaml",
    ConfigPath:  "config/config.yaml",
}

func TestWriteExample(t *testing.T) { helper.WriteExampleFile(t, DefaultConfig()) }
func TestConfigKeysValid(t *testing.T) { helper.ValidateKeys(t) }
```

路径为相对路径，相对于 `go.mod` 所在目录。

#### 生成配置示例（TestWriteExample）

根据 `DefaultConfig()` 结构体自动生成带注释的示例文件：

```bash
go test -v -run TestWriteExample ./internal/config/...
```

生成的示例文件：

```yaml
# 配置示例文件, 复制此文件为 config.yaml 并根据需要修改

# 服务端配置
server:
  addr: ":8080" # 监听地址
  timeout: 30s # 超时时间
```

**工作原理**：通过反射读取结构体的 `koanf` 和 `desc` tag，自动生成完整的 YAML 示例。

#### 校验配置文件（TestConfigKeysValid）

验证配置文件中的所有配置项都在示例文件中定义：

```bash
go test -v -run TestConfigKeysValid ./internal/config/...
```

**用途**：

- 防止配置项拼写错误（如 `servr.addr` 写成 `server.addr`）
- 检测已废弃的配置项
- CI 集成，确保配置文件与代码同步

如果存在无效配置项，测试将失败并列出所有问题项。如果配置文件不存在，测试会自动跳过。

## 模板语法

本库提供 `tmpl` 包用于模板展开，支持环境变量引用和多级 fallback 机制。

```bash
go get github.com/lwmacct/251207-go-pkg-cfgm/pkg/tmpl
```

### 基本语法

| 语法                        | 说明             | 示例                                      |
| --------------------------- | ---------------- | ----------------------------------------- |
| `{{.VAR}}`                  | 直接访问环境变量 | `{{.HOME}}`                               |
| `{{env "VAR"}}`             | env 函数方式     | `{{env "API_KEY"}}`                       |
| `{{env "VAR" "default"}}`   | 带默认值         | `{{env "PORT" "8080"}}`                   |
| `{{.VAR \| default "val"}}` | pipeline 默认值  | `{{.HOST \| default "localhost"}}`        |
| `{{coalesce .A .B "val"}}`  | 多级 fallback    | `{{coalesce .PRIMARY .BACKUP "default"}}` |

### 内置函数

#### env

获取环境变量，支持可选默认值：

- `{{env "VAR"}}` - 不存在返回空字符串
- `{{env "VAR" "default"}}` - 不存在返回默认值

#### default

pipeline 友好的默认值函数：

- `{{.VAR | default "fallback"}}` - 值为空时返回 fallback

#### coalesce

返回第一个非空值：

- `{{coalesce .VAR1 .VAR2 .VAR3 "default"}}` - 按优先级返回

### 使用示例

```go
import "github.com/lwmacct/251207-go-pkg-cfgm/pkg/tmpl"

// 展开模板
result, err := tmpl.ExpandTemplate(`{
  "host": "{{.DB_HOST | default "localhost"}}",
  "port": "{{env "DB_PORT" "5432"}}",
  "key": "{{coalesce .PRIMARY_KEY .BACKUP_KEY "sk-default"}}"
}`)
```

## License

MIT
