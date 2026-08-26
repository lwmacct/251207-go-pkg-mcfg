package cfgm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadReportDescribesSourcesInPriorityOrder(t *testing.T) {
	type Config struct {
		Name  string `json:"name"`
		Debug bool   `json:"debug"`
	}
	path := writeTempConfig(t, "name: from-file\ndebug: false\n")
	t.Setenv("APP_NAME", "from-env")

	cfg, report, err := MustNew(Config{Name: "default", Debug: true}, WithoutDefaultPaths()).
		LoadReport(t.Context(), File(path), Env("APP_"))
	require.NoError(t, err)
	assert.Equal(t, "from-env", cfg.Name)
	assert.False(t, cfg.Debug)
	require.Len(t, report.Sources, 2)
	assert.Equal(t, "file:"+path, report.Sources[0].Name)
	assert.Equal(t, []string{"debug", "name"}, report.Sources[0].Keys)
	assert.Equal(t, "env:APP_", report.Sources[1].Name)
	assert.Equal(t, []string{"name"}, report.Sources[1].Keys)
}

func TestLoadReportDeduplicatesCompositePaths(t *testing.T) {
	type Config struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	path := writeTempConfig(t, "items:\n  - name: one\n  - name: two\n")
	_, report, err := MustNew(Config{}, WithoutDefaultPaths()).LoadReport(t.Context(), File(path))
	require.NoError(t, err)
	require.Len(t, report.Sources, 1)
	assert.Equal(t, []string{"items.name"}, report.Sources[0].Keys)
}
