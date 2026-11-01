// Package plugins is the compile-time catalog of available bot plugins.
package plugins

import (
	"github.com/sajfer/surbot/internal/core"
	"github.com/sajfer/surbot/internal/plugins/fun"
	"github.com/sajfer/surbot/internal/plugins/music"
	"github.com/sajfer/surbot/internal/plugins/rajd"
)

// Catalog returns every plugin that can be enabled, in default start order.
func Catalog() []core.Plugin {
	return []core.Plugin{fun.New(), rajd.New(), music.New()}
}
