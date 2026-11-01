package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// isolate clears the keys these tests read so the developer's shell cannot leak in.
func isolate(t *testing.T) {
	t.Helper()
	for _, k := range []string{"SUR_TOKEN", "SUR_PLUGINS", "SUR_EXAMPLE_KEY"} {
		t.Setenv(k, "")
	}
}

func writeEnv(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	return path
}

func TestLoadReadsPrefixedTokenFromEnvFile(t *testing.T) {
	isolate(t)

	cfg, err := Load(writeEnv(t, "SUR_TOKEN=abc\n"))

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Token != "abc" {
		t.Errorf("Token = %q, want %q", cfg.Token, "abc")
	}
}

func TestLoadReadsUnprefixedTokenFromEnvFile(t *testing.T) {
	isolate(t)

	cfg, err := Load(writeEnv(t, "TOKEN=abc\n"))

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Token != "abc" {
		t.Errorf("Token = %q, want %q", cfg.Token, "abc")
	}
}

func TestLoadPrefersPrefixedKeyWhenFileHasBothForms(t *testing.T) {
	isolate(t)

	cfg, err := Load(writeEnv(t, "TOKEN=plain\nSUR_TOKEN=prefixed\n"))

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Token != "prefixed" {
		t.Errorf("Token = %q, want %q", cfg.Token, "prefixed")
	}
}

func TestLoadReadsTokenFromEnvWhenFileIsMissing(t *testing.T) {
	isolate(t)
	t.Setenv("SUR_TOKEN", "abc")

	cfg, err := Load(filepath.Join(t.TempDir(), "missing.env"))

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Token != "abc" {
		t.Errorf("Token = %q, want %q", cfg.Token, "abc")
	}
}

func TestLoadRealEnvOverridesEnvFile(t *testing.T) {
	isolate(t)
	t.Setenv("SUR_TOKEN", "fromenv")
	t.Setenv("SUR_EXAMPLE_KEY", "envkey")
	path := writeEnv(t, "SUR_TOKEN=fromfile\nSUR_EXAMPLE_KEY=filekey\n")

	cfg, err := Load(path)

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Token != "fromenv" {
		t.Errorf("Token = %q, want %q", cfg.Token, "fromenv")
	}
	if got := cfg.Get("EXAMPLE_KEY"); got != "envkey" {
		t.Errorf("Get(EXAMPLE_KEY) = %q, want %q", got, "envkey")
	}
}

func TestLoadFallsBackToFileWhenEnvVarIsEmpty(t *testing.T) {
	isolate(t)
	t.Setenv("SUR_TOKEN", "")

	cfg, err := Load(writeEnv(t, "SUR_TOKEN=fromfile\n"))

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Token != "fromfile" {
		t.Errorf("Token = %q, want %q", cfg.Token, "fromfile")
	}
}

func TestLoadWithoutTokenReturnsErrNoToken(t *testing.T) {
	isolate(t)

	_, err := Load(filepath.Join(t.TempDir(), "missing.env"))

	if !errors.Is(err, ErrNoToken) {
		t.Errorf("Load() error = %v, want ErrNoToken", err)
	}
}

func TestLoadUnreadableEnvFileReturnsErrorOtherThanNoToken(t *testing.T) {
	isolate(t)
	t.Setenv("SUR_TOKEN", "abc")
	dirAsFile := t.TempDir()

	_, err := Load(dirAsFile)

	if err == nil {
		t.Fatal("Load() error = nil, want a read error")
	}
	if errors.Is(err, ErrNoToken) {
		t.Errorf("Load() error = %v, want an error other than ErrNoToken", err)
	}
}

func TestLoadParsesPluginList(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{name: "trims and drops empties", value: " fun, ,music ", want: []string{"fun", "music"}},
		{name: "empty means all", value: "", want: nil},
		{name: "only separators means all", value: " , ", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			t.Setenv("SUR_TOKEN", "abc")
			t.Setenv("SUR_PLUGINS", tt.value)

			cfg, err := Load(filepath.Join(t.TempDir(), "missing.env"))

			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !reflect.DeepEqual(cfg.Plugins, tt.want) {
				t.Errorf("Plugins = %#v, want %#v", cfg.Plugins, tt.want)
			}
		})
	}
}

func TestGetReadsPluginSettingFromEnv(t *testing.T) {
	isolate(t)
	t.Setenv("SUR_TOKEN", "abc")
	t.Setenv("SUR_EXAMPLE_KEY", "k")

	cfg, err := Load(filepath.Join(t.TempDir(), "missing.env"))

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := cfg.Get("EXAMPLE_KEY"); got != "k" {
		t.Errorf("Get(EXAMPLE_KEY) = %q, want %q", got, "k")
	}
}

func TestGetReadsPluginSettingFromEnvFile(t *testing.T) {
	isolate(t)

	cfg, err := Load(writeEnv(t, "SUR_TOKEN=abc\nSUR_EXAMPLE_KEY=k\n"))

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := cfg.Get("EXAMPLE_KEY"); got != "k" {
		t.Errorf("Get(EXAMPLE_KEY) = %q, want %q", got, "k")
	}
}

func TestFormattingConfigDoesNotRevealToken(t *testing.T) {
	isolate(t)
	cfg, err := Load(writeEnv(t, "SUR_TOKEN=supersecret\n"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		for name, arg := range map[string]any{"pointer": cfg, "value": *cfg} {
			out := fmt.Sprintf(verb, arg)
			if strings.Contains(out, "supersecret") {
				t.Errorf("Sprintf(%q, %s config) = %q, contains the token", verb, name, out)
			}
		}
	}
}
