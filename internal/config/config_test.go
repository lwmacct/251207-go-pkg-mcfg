package config

import (
	"testing"

	"github.com/lwmacct/251207-go-pkg-mcfg/pkg/config"
)

var helper = config.ConfigTestHelper[Config]{
	ExamplePath: "config/config.example.yaml",
	ConfigPath:  "config/config.yaml",
}

func TestGenerateExample(t *testing.T) { helper.GenerateExample(t, DefaultConfig()) }
func TestConfigKeysValid(t *testing.T) { helper.ValidateKeys(t) }
