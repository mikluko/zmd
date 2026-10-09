# zmd

Session navigation for [zmx](https://zmx.sh): a list of places, each holding
the sessions running there and the offer of another.

zmx persists terminal sessions and, by design, provides no windows, tabs or
splits. What it also has no opinion about is *which* session you want. This
fills that gap and nothing else.

```sh
zmd
```

## Two views of places

The picker lists places, in one of two views. `tab` switches between them, and
the picker reopens in whichever you switched to last.

```
repos    every repository below ~ that is not a worktree
dirs     directories outside any repository: where sessions stand, where
         sessions have been, and where the picker was started
```

The two never overlap. A session counts against the repository it stands in,
worktrees included, and only a session outside every repository gets a
directory of its own.

**A place opens in place.** `enter` on it lists what it holds, indented under
it, and `enter` again closes it:

```
▾ 2 acme/api                   ~/src/acme
     ● retry-bug
     ○ flaky-ci        acme/api/rebase-517
       acme/api/docs-pass
       <new session>
▸   acme/web                   ~/src/acme
```

A repository holds every session in it or in its worktrees, then each worktree
no session stands in, then the offer of another session at the repository
itself. Entering a worktree with no session starts one there. A directory holds
the sessions standing exactly in it, then the offer.

**Places with sessions lead**, then the rest by how much and how lately they
have been worked in: a visit a day ago outranks the same number a month ago, as
in [zoxide](https://github.com/ajeetdsouza/zoxide).

**Typing narrows the view.** A query matches places and session titles. A
session that matches brings its place along, open on the matches alone, so a
match never stands without the place it is in. A place that matches only itself
is listed closed, and entering it clears the query and leaves it open under the
cursor.

## The list

```
▾ 2 acme/gateway             ~/src/acme
     ●  retry-bug
     ○  zmd.ghostty.6f2a91c4  acme/gateway/rebase-517
  ↑ ↑↑↑
  │ │││
  │ ││└─ reserved
  │ ││
  │ │└── a place: how many sessions it holds; a session: ○ no client, ● a client
  │ │
  │ └─── what git makes of the directory: bright a repository, dim a worktree of one
  │
  └───── a place: ▸ closed, ▾ open
```

**The status column** is three cells wide whatever it has to say, so every
name below it starts in the same place. The third cell is empty: it is held
for what the agent in a session is doing, which nothing sets yet.

On a place the second cell is a count: how many sessions it holds, `+` past
nine. On a session it says whether a client is attached.

A session goes by its title where something has set one with
`zmx set <name> title=…`, and by its name where nothing has. Where it stands
somewhere other than its place, in a worktree or a directory below, the row
says where.

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
| `enter` | On a place, open or close it. On a session, attach it. On `<new session>` or a worktree with no session, start one there. A session another terminal has open is taken, after asking. |
| `tab` | Switch between the repos and dirs views. |
| `shift-enter` | Mark, for a kill that takes several at once. A marked place stands for every session it holds. |
| `alt-enter` | Start another session where the cursor stands, whatever is already running there. |
| `delete` | Kill the sessions the marked rows stand for, or the row under the cursor, after asking. `y` kills; any other key does not. |
| `alt-delete` | Kill them without asking. |
| `alt-up`, `alt-down` | Scroll the preview. `alt-pgup` and `alt-pgdown` scroll it by the page. |
| `esc` | Leave for the session this pane last attached to. Leave outright where there is none, or where another terminal has it. |

The arrows move the cursor and edit the query, and nothing else: with places
opening in place there is no level to walk up or down.

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

In the view switched to last, with the cursor on the session this pane last
held and its place open around it — which, after a detach, is the session just
left, so its peers are around it. Where that session lives in the other view,
the cursor starts at the top.

A pane that has held nothing has only where it stands to go on, so its place
opens with the cursor on the offer of a session there.

The directory the picker was *run* in is no use for the first: a pane runs it
in a loop, and that loop's shell never chdirs, so it stands where the pane was
opened however much work has happened since. That record is kept per pane,
keyed by the pid of the shell whose loop runs the picker, which is the same
process for as long as the pane is open. Panes sharing one record is panes
going back to a session they were never in.

## How directories are learned

There are some 25,000 directories below `~` within five levels, not counting
hidden ones, and nearly all of them are caches, build output and `Library`, so the dirs view is learned rather
than walked. A directory is learned by working in it: zmx follows a session's
`cd` and reports where it went, and `zmd daemon` credits a visit to every
place a session moves to. Open a session anywhere, `cd` to where the work is,
and the next picker offers it.

Without the daemon the picker learns only when it runs: the place it attaches
or starts in, and any place a running session stands in that it has not seen
before.

The record is `visits.json` in the state directory. A directory gone from disk
is not offered.

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
