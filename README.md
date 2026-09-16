# zmx-nav

Session navigation for [zmx](https://zmx.sh): two lists, one of the places a
session can be and one of the sessions there are.

zmx persists terminal sessions and, by design, provides no windows, tabs or
splits. What it also has no opinion about is *which* session you want. This
fills that gap and nothing else.

```sh
zmx-nav
```

## The two lists

`tab` moves between them, and that is the whole of the navigation.

**Directories** are every repository two levels below the root and every
worktree git records for one. A worktree is offered wherever it physically
sits: beside the repository under `.claude/worktrees/`, or far away under
`~/.claude-squad/worktrees/`.

```
●○  uptime-com.upnats      ~/Forge/uptime-com/upnats
○   mikluko.octant         ~/Forge/mikluko/octant
    mikluko.dotfiles       ~/Forge/mikluko/dotfiles
```

One circle per session in that directory, filled where a client is attached
and hollow where none is. Sessions are counted by the directory they run in
rather than by their name, so a session started by something else — supacode
names its own `supa-<uuid>` — still counts against the repository it sits in.

**Sessions** are what is running, in the same order the circles are drawn, so
the second circle and the second row are the same session.

## What the keys do

| | |
| --- | --- |
| `enter` | Attach to the session here. Where there is none, start one. Where there are several, open the session list for this directory. |
| `alt-enter` | Start another session here, under a label you type. |
| `tab` | Between the two lists. |
| `esc` | Leave. |

There is no `ctrl-enter`: a terminal sends the same byte for it as for
`enter`, and fzf has no name for a key it cannot be told apart from.

Sessions are named `org.repo`, a worktree adds `@label`, and a second session
in the same place adds `+label`:

```
uptime-com.upnats
uptime-com.upnats@2046-eight-token-subjects
uptime-com.upnats+review
```

The org qualifier is there because a repository name repeats across orgs, and
a bare name would send the second one into the first one's session. The
separator is a dot rather than a slash: **zmx names each session's unix socket
after it, so `zmx attach org/repo` creates nothing and says nothing.**

## Where it opens

The cursor starts on the directory the terminal was opened in, so a new pane
is one keystroke away from where it stands. The closest directory wins, which
means a worktree answers with itself rather than with the repository above it,
and a path under neither leaves the cursor at the top.

## The preview

Beside the list where the terminal is wide, below it where it is not, and left
out entirely where what remained of the list would be unreadable. A directory
previews as a listing, a session as its own scrollback through `zmx history`.

## Install

```sh
brew install mikluko/tap/zmx-nav
```

Or from source:

```sh
go install github.com/mikluko/zmx-nav@latest
```

Needs `zmx` on `PATH`, and `lsd` for the directory preview. fzf is a library
here rather than a command, so it does not have to be installed.

## Configuration

| | |
| --- | --- |
| `--root DIR` | Where repositories live. |
| `ZMX_NAV_ROOT` | The same, as an environment variable. |

The default is `~/Forge`. The layout expected below it is `<org>/<repo>`.

## Holding a pane open

Run it in a loop, and the pane outlives the sessions it shows: zmx's client
takes `ctrl+\` before the PTY sees it, so a detach lands back here from inside
whatever the session was running.

```
# Ghostty
command = /bin/zsh -lc 'while zmx-nav; do :; done'
```

## Why it reads git rather than running it

Worktrees come from `.git/worktrees/<id>/gitdir`, the record git writes, not
from `git worktree list`. At 84 repositories that is 84 directory reads
instead of 84 subprocesses, and it finds worktrees outside the repository the
same way as the ones beside it.

## License

MIT.
