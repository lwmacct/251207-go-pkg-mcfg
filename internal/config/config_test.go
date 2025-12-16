package config

import (
	"testing"

	"github.com/lwmacct/251207-go-pkg-mcfg/pkg/mcfg"
)

var helper = mcfg.ConfigTestHelper[Config]{
	ExamplePath: "config/config.example.yaml",
	ConfigPath:  "config/config.yaml",
}

func TestWriteExample(t *testing.T)    { helper.WriteExampleFile(t, DefaultConfig()) }
func TestConfigKeysValid(t *testing.T) { helper.ValidateKeys(t) }
