# zmx-deck

Session navigation for [zmx](https://zmx.sh): one list, holding the sessions
there are and the repositories a session can be started in.

zmx persists terminal sessions and, by design, provides no windows, tabs or
splits. What it also has no opinion about is *which* session you want. This
fills that gap and nothing else.

```sh
zmx-deck
```

## The list

Typing is the navigation. A query narrows the sessions and the repositories at
once, and the kind of the row it leaves under the cursor is what decides what
a key does to it.

```
●   Forge.mikluko.zmx-deck         ~/Forge/mikluko/zmx-deck
○   Forge.mikluko.zmx-deck+review  ~/Forge/mikluko/zmx-deck
●   supa-7f3a2b                   ~/Forge/uptime-com/upnats
●○  [repo] zmx-deck                ~/Forge/mikluko
    [repo] 2046-eight             ~/Forge/mikluko/zmx-deck/.claude/worktrees
    [repo] dotfiles               ~/Forge/mikluko
```

**Sessions** come first, in name order, each with a circle: filled where a
client is attached, hollow where none is.

**Repositories** are every directory below `~` holding a `.git`, which a
worktree holds as much as a repository does. Each carries one circle per
session running in it. Sessions are counted by the directory they run in
rather than by their name, so a session started by something else — supacode
names its own `supa-<uuid>` — still counts against the repository it sits in.

A repository's name is its basename and its path is the parent, so the two
together are the path written once. Both are searched: the query is matched
against the whole line.

## What the keys do

| | |
| --- | --- |
| `enter` | Attach the session. On a repository, attach the one session there or start one where there is none. Where there are several, narrow the list to them. |
| `alt-enter` | Start another session where the cursor stands, under a label you type. |
| `ctrl-n` | Start a session named after what you typed, whether or not it matched anything. |
| `tab` | Mark, for a kill that takes several at once. Enter clears the marks rather than acting on them. |
| `delete` | Kill the marked sessions, or the one under the cursor, after asking. |
| `ctrl-delete` | Kill them without asking. |
| `esc` | Leave. |

Enter on a query that matched nothing starts a session under that name, in the
directory the pane stands in. `ctrl-n` is the same thing forced: it names the
session after the query even when a row is highlighted.

A repository running several sessions is an ambiguity one keystroke cannot
settle, so `enter` and `delete` both answer it by narrowing the list to those
sessions rather than by guessing. The narrowing is a query like any other, and
backspace clears it.

Backspace is not a kill. A filter typed to find the session to kill would
otherwise leave every correction one keystroke from killing whatever the
cursor had been left on.

There is no `ctrl-enter`: a terminal sends the same byte for it as for
`enter`, and fzf has no name for a key it cannot be told apart from.

## How sessions are named

A session started for a directory is named after the path, dotted, taken
relative to `~` where it is below it and to the root where it is not. A second
session in the same place adds `+label`.

```
~/Forge/mikluko/notes   Forge.mikluko.notes
/etc/nginx              etc.nginx
                        Forge.mikluko.notes+review
```

The whole path is there because a repository name repeats across orgs, and a
bare name would send the second one into the first one's session. The
separator is a dot rather than a slash: **zmx names each session's unix socket
after it, so `zmx attach org/repo` creates nothing and says nothing.**

## Where it opens

The cursor starts on the row closest to the directory the terminal was opened
in, so a new pane is one keystroke away from where it stands. The closest
directory wins, which means a worktree answers with itself rather than with
the repository above it, and a directory that has a session answers with the
session rather than with the offer to start a second.

## The preview

Beside the list where the terminal is wide, below it where it is not, and left
out entirely where what remained of the list would be unreadable. A session
previews as its own scrollback through `zmx history`, at the end of it; a
repository as its history through `git log --graph`.

## Install

```sh
brew install mikluko/tap/zmx-deck
```

Or from source:

```sh
go install github.com/mikluko/zmx-deck@latest
```

Needs `zmx` on `PATH`, and `git` for the repository preview. fzf is a library
here rather than a command, so it does not have to be installed.

## Holding a pane open

Run it in a loop, and the pane outlives the sessions it shows: zmx's client
takes `ctrl+\` before the PTY sees it, so a detach lands back here from inside
whatever the session was running.

```
# Ghostty
command = /bin/zsh -lc 'while zmx-deck; do :; done'
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
