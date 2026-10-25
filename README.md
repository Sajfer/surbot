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
plugins; the Helm chart does not set it. The music plugin stops playback (yt-dlp and ffmpeg) and
leaves voice in about 1-2s, so shutting down while music plays does not hit the drain timeout.

### Available plugins

| Plugin | Commands |
| --- | --- |
| `legacy` | `ping`, `chuck`, `roll`, `rajd` (temporary; replaced by separate plugins later) |
| `music` | `play`, `playing`, `queue`, `shuffle`, `skip`, `stop`, `clearQueue`, `disconnect` |

```
SUR_PLUGINS=music          # music commands (and help) only
SUR_PLUGINS=legacy,music   # same as leaving it unset
```

### Music

Each guild has its own player: one queue, one goroutine that joins the voice channel and plays
the queue in order, and a 5 minute idle timer after which the bot leaves. `!play` returns as soon
as the songs are queued; joining and playback happen in the background, and a failed join is
reported in the text channel (`Failed to play song`) with the queue kept.

- `!play <query>` uses the whole query, spaces included. Without arguments it shows its usage.
  When the bot is not in voice, the author has to be in a voice channel (`Join a voice channel
  first.`). A YouTube video, playlist (`list=`), Spotify link or free-text search is accepted.
- `!queue` shows the current song first, then one numbered line per song, at most 20 lines and
  then `… and N more`.
- `!stop` stops the current song, keeps the queue and stays in voice; the next `!play` plays the
  old queue first. `!disconnect` stops and leaves voice, also keeping the queue. `!skip` on the
  last song starts the idle timer like any other end of playback.
- `!play` moves the bot to the requester's voice channel when nothing is playing (idle or
  stopped); while a song is playing the bot stays where it is and the song is only queued.
- Music commands in a direct message answer `Music commands only work in a server.`

Known limitations:

- Joining a voice channel cannot be cancelled and can take up to about 11s, so shutting down
  during a join can hit the 10s teardown limit.
- The listening status is bot-wide, not per guild.
- Spotify playlists and albums queue only their first track, and Spotify links must be in the
  `?si=` form (a link without a query string is not recognised).
- Hint texts hard-code the `!` prefix.
- Every YouTube lookup creates its own metadata client, because the library's client is not safe
  for concurrent use and sharing one behind a lock would let one guild's large playlist block
  every other guild. The cost is a few extra requests per lookup (the client's visitor ID and
  player cache are fetched again each time). A large playlist is still resolved video by video
  and delays only the `!play` that asked for it. Lookups are cancelled at shutdown.
