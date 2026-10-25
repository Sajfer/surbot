# surbot

A discord bot mainly for playing music in a channel

Written i Golang

## Running

```
go run main.go          # or: make build && bin/surbot
go run main.go -p '?'   # different command prefix (default !)
```

Settings are read from the environment and from a `.env` file in the working directory.
Real environment variables override `.env`; in `.env` the `SUR_` prefix is optional (`SUR_TOKEN`
and `TOKEN` both work, the prefixed key wins if both are present).

| Key | Purpose |
| --- | --- |
| `SUR_TOKEN` | Discord bot token (required) |
| `SUR_YOUTUBE_API` | YouTube Data API key (search) |
| `SUR_SPOTIFY_CLIENTID`, `SUR_SPOTIFY_CLIENTSECRET` | Spotify credentials (optional, Spotify links are disabled without them) |
| `SUR_PLUGINS` | Comma-separated plugins to enable, in start order. Unset or empty enables all. An unknown or duplicate name stops the bot at startup. |
| `SUR_DEBUG` | Any non-empty value turns on debug logging |

## Commands

A command is the prefix, the command name, a space, then its arguments: `!roll d20`,
`!play <url>`. Names are case-sensitive and must be followed by a space (or end of message), so
`!rolld20` and `!play<url>` are ignored. Commands that take no arguments ignore any extra words
(`!ping foo` answers `Pong!`); `!roll` and `!play` use everything after the name (`!roll d6 please`
answers with the usage text, `!play` uses all words as the query). Bots never trigger commands.
`!help` lists what is registered.

| Command | Arguments | Description |
| --- | --- | --- |
| `!help` | | Show the command list |
| `!ping` | | Responds with Pong! |
| `!chuck` | | Responds with a Chuck Norris joke |
| `!play` | `<youtube link\|spotify link\|search>` | Queue a song and start playing in your voice channel |
| `!playing` | | Show the current song |
| `!queue` | | Show the queue |
| `!shuffle` | | Shuffle the queue |
| `!skip` | | Skip the current song |
| `!stop` | | Stop playing |
| `!clearQueue` | | Clear the queue |
| `!disconnect` | | Leave the voice channel |
| `!roll` | `d6\|d10\|d20\|d100` | Roll a die |
| `!rajd` | | Post this week's rajd days with reactions |

## Architecture / plugins

`internal/core` owns the Discord connection, the command registry and prefix router, the
built-in `help` command and the plugin lifecycle. It knows nothing about features. Each feature
is a plugin (`internal/plugins/<name>`) implementing `core.Plugin`:

- `Setup(ctx, Registrar)` registers commands (`Registrar.Command`) and Discord event listeners
  (`Registrar.AddHandler`) and reads settings from `Registrar.Services().Config`.
- `Teardown(ctx)` releases everything the plugin started. Core removes the plugin's listeners
  first.

Plugins are compiled in: add one by implementing `core.Plugin` and listing it in
`internal/plugins/plugins.go`. `SUR_PLUGINS` selects which of them run.

On shutdown (SIGINT/SIGTERM) core stops routing, waits for running commands (up to 10s), tears
the plugins down in reverse order (up to 10s each), then closes the Discord connection. A second
signal during that time ends the process immediately. Worst case is 10s plus 10s per plugin,
which can exceed Kubernetes' default `terminationGracePeriodSeconds` of 30s with three or more
plugins; the Helm chart does not set it.

Currently every command runs through the temporary `legacy` plugin, which wraps `pkg/surbot`.
Known limitations of it:

- Teardown does not stop playback or leave voice; the voice connection drops when the gateway
  closes.
- `!play` blocks its command until the whole queue has played, so shutting down while music plays
  always hits the 10s drain timeout: the command is abandoned and the process exits.
- Idle-timer goroutines are not owned by anyone.
- Spotify login happens in `Setup` and is not bounded by its context.
