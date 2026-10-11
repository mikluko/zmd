# zmd

Session navigation for [zmx](https://zmx.sh): start a session in a checkout,
or switch to one already running.

zmx persists terminal sessions and, by design, provides no windows, tabs or
splits. What it also has no opinion about is *which* session you want. This
fills that gap and nothing else.

```sh
zmd
```

## Three views

`tab` cycles through them, and the picker reopens in whichever you switched to
last.

**The checkouts.** Every repository and worktree below `~`, one row each,
marked by which it is and whether sessions stand in it. `enter` on one with
none starts a session there, which is what the picker is mostly for; on one
holding sessions it lists them, with the offer of another last, and `tab`
goes back to the checkouts. Checkouts a
session has worked in lead, the latest first; the rest follow by path.

**The sessions.** Every session, wherever it stands, the one zmd attached
latest first; a session zmd never attached ranks by when zmx started it.
`enter` attaches it.

**Browse.** For the rarer session somewhere that is not a checkout yet, say
where a new repository is about to be made. The picker stands in one
directory, opening at `~`, and
lists the sessions standing there, the offer of another, and the directories
below it, four levels down:

```
~/Forge/acme>
     ..                                  enter: stand in the directory above
   ● retry-bug
     <new session>                       enter: start a session here
 ● ~/Forge/acme/api                       enter: stand in it
   ~/Forge/acme/api/internal
   ~/Forge/acme/web
```

`enter` on a directory stands in it, and `..` stands in the one above. A
directory counts every session standing in it or below it, so work deep in a
tree shows from its top. A repository or a worktree is a directory like any
other, with a mark of its own.

Directories a session has worked in lead, the latest first, since that is
where the next session most likely goes; the rest follow by path. The tree
leaves out hidden directories, caches, build output, `Library` and
application bundles, save a directory a session stands in or has worked in.

**Typing narrows the view**, matching a checkout's or directory's path, or a
session's title, name and place. The way up and the offer of a session are
not things a query looks for, and drop out while one is typed.

## The list

```
 ●   retry-bug               ~/src/acme/gateway
○  ~/Forge/acme/api
 ↑↑↑
 │││
 ││└─ reserved
 ││
 │└── ● all attached, ◐ some attached, ○ none attached, blank no session
 │
 └─── what git makes of the directory:  a repository's own checkout,  a worktree cut from one
```

**The status column** is three cells wide whatever it has to say, so every
name below it starts in the same place. The third cell is empty: it is held
for what the agent in a session is doing, which nothing sets yet.

On a checkout or directory the second cell stands for every session there:
filled where every one has a client, hollow where none has, half filled where
some have. How many there are is not what it answers.

A session goes by its title where something has set one with
`zmx set <name> title=…`, and by its name where nothing has.

## What the keys do

| | |
| --- | --- |
| `enter` | On a checkout, start a session there if none stands in it, or list those that do, with the offer of another. On a session, attach it, taking it after asking where another terminal has it open. In browse: on a directory, stand in it; on `..`, stand in the one above; on `<new session>`, start one where the picker stands. |
| `tab` | Cycle through checkouts, sessions and browse. |
| `shift-enter` | Mark, for a kill that takes several at once. A marked directory stands for every session in it or below it. |
| `alt-enter` | Start another session where the cursor stands, whatever is already running there. |
| `delete` | Kill the sessions the marked rows stand for, or the row under the cursor, after asking. `y` kills; any other key does not. |
| `alt-delete` | Kill them without asking. |
| `pgup`, `pgdown` | Move the cursor a screenful of lines. |
| `alt-up`, `alt-down` | Scroll the preview. `alt-pgup` and `alt-pgdown` scroll it by the page. |
| `esc` | Leave for the session this pane last attached to. Leave outright where there is none, or where another terminal has it. |

The arrows move the cursor and edit the query, and nothing else: going up is
`enter` on `..`, which leaves them free to correct what was typed.

The hint line offers only the keys that would do something where the cursor
stands: no `⌦ kill` where nothing is running, and `⏎ start` rather than
`⏎ attach` where entering means making one.

`shift-enter` is a key of its own only because the picker asks the terminal
for the kitty keyboard protocol, which Ghostty and kitty implement. A terminal without it sends `shift-enter` as `enter`, and marking
is then out of reach.

## One terminal to a session

zmx lets any number of clients attach to one session, and they mirror each
other, keystrokes and all. Nothing here ever does that. A session another
terminal has open is taken rather than joined: `enter` asks first, and then
that terminal's client is dropped and this one attaches.

Escape refuses it too. Going back to a session someone else is in would mirror
it, so a record naming one is read past, and where that leaves nothing the
picker exits rather than attaching.

A filled circle is a session with a client, whoever holds it. The one this
terminal holds is not one it can collide with, and `zmx list` marks it: a
session the picker was opened from is still an ordinary enter away.

zmx has no read-only client, or enter would attach as one. `zmx attach` takes
`--labels` and nothing else, and `zmx history --vt` piped into `zmx tail` is a
second implementation of the client rather than a flag on the real one.
`zmx detach` takes no session name either, reading it from `ZMX_SESSION`,
which is how a session is taken.

## How sessions are named

```
zmd.ghostty.6f2a91c4
└┬┘ └──┬──┘ └───┬──┘
 │     │        └─ four bytes of randomness, hex
 │     └────────── the terminal it was made in, read from TERM_PROGRAM
 └──────────────── what made it, so a session zmd started is told from one
                   it merely found
```

The name is an address and says nothing about the work. What a session is for
is metadata kept against this name, and the name only has to be unique and to
say where it came from.

zmx names each session's unix socket after it, flat in one directory, so a
name carrying a separator creates nothing and reports nothing. These carry
none.

## Where it opens

The browse view opens at `~`, always. The sessions view opens with the
cursor on the session this pane last held, which after a detach is the one
just left.

That record is kept per pane, keyed by the pid of the shell whose loop runs
the picker, which is the same process for as long as the pane is open. Panes
sharing one record is panes going back to a session they were never in.

## How directories are learned

Which directories lead is learned from where sessions work: zmx follows a
session's `cd` and reports where it went, and `zmd daemon` credits a visit to
every directory a session moves to. Open a session anywhere, `cd` to where the
work is, and the next picker lists that directory first, even a hidden one the
tree would leave out.

Without the daemon the picker learns only when it runs: the directory it
attaches or starts in, and any directory a running session stands in that it
has not seen before.

The records are `visits.json` and `attaches.json` in the state directory.

## The preview

Beside the list where the terminal is wide, above it where it is not, and left
out entirely where what remained of the list would be unreadable. A session
previews as its own scrollback through `zmx history`, at the end of it; a
repository as its history through `git log --graph`, from the newest commit;
any other directory as what it holds, directories first.

## The daemon

```sh
zmd daemon
```

Optional, and the picker never waits on it. It reads what is running every
second and credits a visit to every directory a session moves to, which is
what puts the directories worked in first; see
[How directories are learned](#how-directories-are-learned).

It also walks the checkouts below `~` and holds the result, so a picker asks
rather than walks: 290ms before a single row was drawn, against 3ms. A picker
that cannot reach it walks for itself.

The socket sits beside the rest of the state, or wherever `ZMD_DECK_DIR`
says — an SSH `RemoteForward` being the case that needs to choose.

## Install

```sh
brew install mikluko/tap/zmd
```

Or from source:

```sh
go install github.com/mikluko/zmd@latest
```

Needs `zmx` on `PATH`, and `git` for the repository preview. The picker is
drawn by zmd itself, so fzf does not have to be installed.

## Holding a pane open

Run it in a loop, and the pane outlives the sessions it shows: zmx's client
takes `ctrl+\` before the PTY sees it, so a detach lands back here from inside
whatever the session was running.

```
# Ghostty
command = /bin/zsh -lc 'while zmd; do :; done'
```

## Why a `.git` is enough

Repositories are found by walking `~` and stat'ing a `.git` in each directory,
never by running git. A repository has a `.git` directory and a linked
worktree has a `.git` file, so one stat finds both and a worktree is a row
like any other.

The walk does not descend into hidden directories, with `.claude` the one
exception: it is where a worktree is kept beside the repository it belongs to.

## License

MIT.
