package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lwmacct/251207-go-pkg-cfgm/pkg/cfgm"
	"github.com/stretchr/testify/require"
)

var files = cfgm.ConfigFiles[config]{
	Manager:     manager,
	ExampleFile: "examples/config/config.example.yaml",
	RuntimeFile: "examples/config/config.yaml",
}

func TestWriteConfigExample(t *testing.T) {
	files.WriteExample(t)
}

func TestRuntimeConfigValid(t *testing.T) {
	files.ValidateRuntimeConfig(t)
}

func TestStrictAndPermissiveValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: example\nextra: true\n"), 0o600))

	_, err := manager.Load(t.Context(), cfgm.File(path))
	require.ErrorContains(t, err, `unknown object member name "extra"`)

	permissive := cfgm.New(defaultConfig(), cfgm.WithoutDefaultPaths(), cfgm.AllowUnknownKeys())
	_, err = permissive.Load(t.Context(), cfgm.File(path))
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(path, []byte("timeout: invalid\n"), 0o600))
	_, err = permissive.Load(t.Context(), cfgm.File(path))
	require.ErrorContains(t, err, `invalid duration "invalid"`)
}
