package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/lwmacct/251207-go-pkg-cfgm/pkg/cfgm"
)

type memorySource struct {
	values map[string]any
}

func (s memorySource) Name() string {
	return "memory"
}

func (s memorySource) Load(ctx context.Context, schema cfgm.Schema) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !schema.Has("service.endpoint") {
		return nil, errors.New("service.endpoint is not present in the target schema")
	}
	return s.values, nil
}

type config struct {
	Service struct {
		Endpoint string `json:"endpoint" desc:"服务端地址"`
		Token    string `json:"token"    desc:"访问令牌"`
	} `json:"service" desc:"服务配置"`
}

func main() {
	source := memorySource{values: map[string]any{
		"service": map[string]any{
			"endpoint": "https://memory.example.com",
			"token":    "secret",
		},
	}}
	manager := cfgm.MustNew(config{}, cfgm.WithoutDefaultPaths())
	loaded, report, err := manager.LoadReport(context.Background(), source)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(
		os.Stdout,
		"endpoint=%s token_set=%t source=%s keys=%v\n",
		loaded.Service.Endpoint,
		loaded.Service.Token != "",
		report.Sources[0].Name,
		report.Sources[0].Keys,
	)
}
