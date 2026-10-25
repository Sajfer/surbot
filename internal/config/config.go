// Package config loads the bot's settings from the environment and an optional .env file.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// ErrNoToken is returned by Load when no Discord token is configured.
var ErrNoToken = errors.New("no Discord token: set SUR_TOKEN in the environment or SUR_TOKEN/TOKEN in .env")

const envPrefix = "sur"

// Config holds the core settings; plugin settings are read with Get.
type Config struct {
	Token   string
	Plugins []string // enabled plugin names in order; nil means all
	v       *viper.Viper
}

// String redacts the token so a logged Config cannot leak it.
func (c Config) String() string {
	return fmt.Sprintf("config.Config{Token:[redacted] Plugins:%v}", c.Plugins)
}

// GoString redacts the token for %#v.
func (c Config) GoString() string { return c.String() }

// Load reads the environment and, when envFile exists, that file. Real
// environment variables win over the file. In the file a SUR_ prefix on keys
// is optional, and the prefixed key wins if both forms are present.
func Load(envFile string) (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix(envPrefix)
	v.AutomaticEnv()
	for _, key := range []string{"TOKEN", "PLUGINS"} {
		if err := v.BindEnv(key); err != nil {
			return nil, fmt.Errorf("bind %s: %w", key, err)
		}
	}

	if _, err := os.Stat(envFile); err == nil {
		fileValues, err := readEnvFile(envFile)
		if err != nil {
			return nil, err
		}
		if err := v.MergeConfigMap(fileValues); err != nil {
			return nil, fmt.Errorf("merge %s: %w", envFile, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat %s: %w", envFile, err)
	}

	cfg := &Config{Token: v.GetString("token"), Plugins: splitList(v.GetString("plugins")), v: v}
	if cfg.Token == "" {
		return nil, ErrNoToken
	}
	return cfg, nil
}

// Get returns a plugin setting, e.g. Get("YOUTUBE_API") reads SUR_YOUTUBE_API.
func (c *Config) Get(key string) string {
	return c.v.GetString(strings.ToLower(key))
}

func readEnvFile(path string) (map[string]any, error) {
	fv := viper.New()
	fv.SetConfigFile(path)
	fv.SetConfigType("env")
	if err := fv.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	const prefix = envPrefix + "_"
	values := make(map[string]any)
	for _, key := range fv.AllKeys() {
		if !strings.HasPrefix(key, prefix) {
			if _, set := values[key]; !set {
				values[key] = fv.GetString(key)
			}
			continue
		}
		values[strings.TrimPrefix(key, prefix)] = fv.GetString(key)
	}
	return values, nil
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
