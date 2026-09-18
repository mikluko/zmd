# zmd

Session navigation for [zmx](https://zmx.sh): one list of places, holding the
sessions there are and the directories a session can be started in.

zmx persists terminal sessions and, by design, provides no windows, tabs or
splits. What it also has no opinion about is *which* session you want. This
fills that gap and nothing else.

```sh
zmd
```

## The stack

The picker is a stack of rungs. `right` pushes, `left` pops back to wherever
it was pushed from, and `enter` is right's alias: descending is one motion and
it has one key too many already.

```
sessions      every session running            ⌃g ↓        where it opens
repos         one row per checkout             →  ↓        only if it has worktrees
checkouts     the repository, then its worktrees
sessions      the ones standing in that checkout
```

**Sessions are the ground floor**, and what the picker opens on. They are what
a switch is between, and what a scan is over: an agent wanting attention and a
shell left running are both facts about sessions, and looking for either in a
list of repositories means reading a list twenty times longer and then
descending to learn which session it was.

**The repositories above are how a session is started somewhere new**, which
is the rarer thing and the only thing they are for. A checkout that already
holds sessions pushes straight back down to them, filtered to itself, rather
than listing them a second way.

Typing narrows whatever rung you are on. The prompt says which that is.

## The list

```
 ●   retry-bug              acme/gateway   ~/src/acme/gateway
 ○   zmd.ghostty.6f2a91c4   acme/gateway   ~/src/acme/gateway
 ●   zmd.ghostty.b71e0d55   widget/api     ~/src/widget/api
 ↑↑↑
 │││
 ││└─ reserved
 ││
 │└── this session: ○ no client, ● a client
 │
 └─── what git makes of the directory: bright a repository, dim a worktree of one
```

**The status column** is three cells wide whatever it has to say, so every
name below it starts in the same place. The third cell is empty: it is held
for what the agent in a session is doing, which nothing sets yet.

On the repositories rung the second cell is a count instead — how many
sessions are alive at or below the row, `+` past nine. At a level where no
single session can be acted on, how much is alive below is the whole of what
the mark is read for, and the repositories holding something lead the list.

A session row says what it is, what git calls where it is, and where that is.
The middle earns its column because the two differ: a checkout named
`acme/workspace` can stand at `~/src/other`, which the name does not give up.
It goes blank where it would only repeat the path, `~` being the case.

**A place's name** is what git says it is: `owner/name`, read off the
checkout's remote. The path is where a checkout happens to sit and says
nothing reliable about what it holds — a directory called `uptime-com` can
hold `acme/workspace`, and one renamed on disk keeps the name it was cloned
under. A worktree goes by its repository and what is left of its own basename
once the repository's name is taken off the front, that prefix being how the
tools that cut worktrees name them.

A directory git knows nothing about, or a checkout with no remote, goes by its
directory under its parent, which is all there is to go on.

```
a checkout       acme/api
its worktree     acme/api/rebase-517
neither          Documents/notes
```

The remotes are read out of each `.git/config` rather than by opening the
repository: go-git refuses one whose `core.repositoryformatversion` carries an
extension it does not implement, and `worktreeconfig` is one git sets on
exactly the repositories worktrees have been cut from.

The marks are Nerd Font and Geometric Shapes glyphs. Berkeley Mono carries
none of them, including the circles, which the terminal has been drawing from
its fallback font all along.

## What the keys do

| | |
| --- | --- |
| `enter`, `right` | Descend. On a session, attach it. On a repository with worktrees, list them. On a checkout, its sessions, or start one where it has none. |
| `left`, `backspace` | Back a rung. With a query typed they correct it instead. |
| `ctrl-g` | From the sessions rung, the repositories. |
| `alt-enter` | Start another session where the cursor stands, whatever is already running there. |
| `tab` | Mark, for a kill that takes several at once. Enter clears the marks rather than acting on them. |
| `delete` | Kill the sessions the marked rows stand for, after asking. |
| `ctrl-delete` | Kill them without asking. |
| `esc` | Leave for the session this pane last attached to. Leave outright where there is none, or where another terminal has it. |

The hint line offers only the keys that would do something where the cursor
stands: no `⌦ kill` where nothing is running, `⏎ start` rather than `⏎ attach`
where entering means making one, and no `← back` on the ground floor. fzf
rewrites it per row through the same `focus` transform that sets the preview's
follow mode, since between turns of the loop nothing of this program is
running to be asked.

There is no `ctrl-enter`: a terminal sends the same byte for it as for
`enter`, and fzf has no name for a key it cannot be told apart from. There is
no `shift-enter` either, for the same reason: a terminal sends it as a kitty
keyboard sequence, which fzf does not decode.

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

On the sessions rung, with the cursor on the session this pane last held —
which, after a detach, is the one just left, so its peers are around it.

A pane that has held nothing has only where it stands to go on, so it opens
one rung in, narrowed to that directory, with the cursor on the offer of a
session there. `left` pops back to every session there is.

```
acme/gateway>
 ●   retry-bug        acme/gateway   ~/src/acme/gateway
     <new session>    acme/gateway   ~/src/acme/gateway   ← the cursor starts here
```

The directory the picker was *run* in is no use for this: a pane runs it in a
loop, and that loop's shell never chdirs, so it stands where the pane was
opened however much work has happened since. That record is kept per pane,
keyed by the pid of the shell whose loop runs the picker, which is the same
process for as long as the pane is open. Panes sharing one record is panes
going back to a session they were never in.

## The preview

Beside the list where the terminal is wide, below it where it is not, and left
out entirely where what remained of the list would be unreadable. A session
previews as its own scrollback through `zmx history`, at the end of it; a
repository as its history through `git log --graph`.

## The daemon

```sh
zmd daemon
```

Optional, and the picker never waits on it. It holds the walk warm and a
picker asks instead of walking: 290ms before a single row was drawn, against
3ms, on every detach.

A picker that cannot reach it walks for itself, which is what keeps this an
optimisation rather than a dependency and what lets the daemon be restarted
under a running picker.

The socket sits beside the rest of the state, or wherever `ZMD_DECK_DIR`
says — an SSH `RemoteForward` being the case that needs to choose.

## Install

```sh
brew install acme/tap/zmd
```

Or from source:

```sh
go install github.com/mikluko/zmd@latest
```

Needs `zmx` on `PATH`, and `git` for the repository preview. fzf is a library
here rather than a command, so it does not have to be installed.

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
