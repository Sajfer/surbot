package core

import (
	"context"
	"strings"
)

const coreOwner = "core"

func helpCommand(prefix string, reg *registry) Command {
	return Command{
		Name:        "help",
		Description: "Show this list",
		Handler: func(_ context.Context, c *Context) error {
			return c.Reply(helpText(prefix, reg.sorted()))
		},
	}
}

func helpText(prefix string, cmds []Command) string {
	var b strings.Builder
	b.WriteString("Available commands:\n")
	for _, cmd := range cmds {
		b.WriteString("**" + prefix + cmd.Name + "** ")
		if cmd.Usage != "" {
			b.WriteString(cmd.Usage + " ")
		}
		b.WriteString("— " + cmd.Description + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}
