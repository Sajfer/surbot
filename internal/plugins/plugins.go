// Package plugins is the compile-time catalog of available bot plugins.
package plugins

import (
	"gitlab.com/sajfer/surbot/internal/core"
	"gitlab.com/sajfer/surbot/internal/plugins/legacy"
	"gitlab.com/sajfer/surbot/internal/plugins/music"
)

// Catalog returns every plugin that can be enabled, in default start order.
func Catalog() []core.Plugin {
	return []core.Plugin{legacy.New(), music.New()}
}
