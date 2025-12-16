// Package tmpl 提供配置模板展开功能。
//
// 与 Taskfile 模板语法对齐，支持灵活的环境变量访问。
//
// # 设计参考
//
//   - Taskfile 模板语法: https://taskfile.dev/docs/reference/templating
//   - Taskfile 环境变量: https://taskfile.dev/docs/reference/environment
//   - Sprig 模板函数: https://github.com/Masterminds/sprig
//
// # 核心设计原则
//
//  1. 环境变量通过 {{.VAR}} 自动可用（Taskfile 风格）
//  2. env 函数可选，在变量名冲突时使用
//  3. 管道友好：{{.VAR | default "fallback"}}
//  4. 多级 fallback：coalesce 函数支持优雅降级链
//
// # 支持的函数
//
//   - env: 获取环境变量 {{env "VAR"}} 或 {{env "VAR" "default"}}
//   - default: 管道默认值 {{.VAR | default "fallback"}}
//   - coalesce: 返回第一个非空值 {{coalesce .VAR1 .VAR2 "default"}}
//
// 详见 [ExpandTemplate] 文档。
package tmpl
