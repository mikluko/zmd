# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/2.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] - 2026-10-11

### Changed

- Renamed to `zmd`, from `zmx-nav`. The old name described navigating between lists, which is neither what the tool does nor what it is for; this one is zmx's deck, the place you stand to see the whole of a thing and take it somewhere. The module path and the command both move, and `go install github.com/mikluko/zmx-nav` installs nothing after this.
- One picker, with three views that `tab` cycles through, reopening in whichever was switched to last. The checkouts view lists every repository and worktree below `~`, worked-in first, and `enter` starts a session in one with none, which the picker is mostly for, or lists the sessions of one that has them, with the offer of another last. The sessions view lists every session, the one zmd attached latest first. Browse stands in one directory, opening at `~`, and lists the sessions there, the offer of another, and the directories four levels below, for a session somewhere that is not a checkout yet; `enter` on a directory stands in it and on `..` in the one above. The arrows move the cursor and edit the query, and nothing else.
- A query matches a directory's path and name, or a session's title, name and place; the way up and the offer of a session drop out while one is typed.
- Sessions are named `zmd.<terminal>.<shortid>` — `zmd.ghostty.6f2a91c4` — and the name is an address rather than a description: what made the session, the terminal it was made in read from `TERM_PROGRAM`, and four bytes of randomness. A session zmd started is told from one it merely found, and the name says nothing about the work. What a session is for is metadata, `zmx set <name> title=…`, which rides on the record `zmx list` already writes; the handle is the fallback and what a session goes by where nothing has labelled it.
- A status column three cells wide ahead of every name, so a name starts in the same place on every line. The first says what git makes of the directory, a branch for a repository's own checkout and a commit for a worktree cut from one; the second is filled where a session, or every session a row stands for, has a client, hollow where none has, and half filled where some have; the third is held for what the agent in a session is doing and is blank until something sets it. Being fixed is what lets the column carry colour: `text/tabwriter` counts an escape byte as a character, so the column is written past the alignment rather than through it.
- The picker is drawn by zmd itself rather than by fzf, which keeps only its matcher here. The marks, the cursor and what is open live in one process for as long as the picker is up, so a kill happens in place, where fzf had to be torn down and started again for it. It reads what is running every second, believing a change once two readings agree since reading a preview attaches a client for as long as the read takes, and asks for the repositories every thirty seconds, so a session started or killed elsewhere, or a repository cloned, shows while the picker is up. It keeps fzf's look: its dark theme, the rounded border, the count line, and a scrollbar beside the list and the preview where either overflows. The split between them is decided on every draw, so a preview a small terminal had no room for comes back when it grows. A kill asks on the hint line, `y` for yes and any other key for no.
- An attach from inside a session with no client attached is refused: zmx reads it as that session's client switching to the target, and ends a session with no client to switch.
- A `zmx list` that fails is reported rather than shown as nothing running, and a preview that cannot be read shows why rather than an empty pane.
- `pgup` and `pgdown` move the cursor a screenful of lines; `alt-pgup` and `alt-pgdown` scroll the preview.
- `shift-enter` marks, where `tab` did: `tab` switches views now. It is a key distinct from `enter` because the picker asks the terminal for the kitty keyboard protocol, which bubbletea v2 speaks.
- The hint line offers only the keys that would do something where the cursor stands: no `⌦ kill` where nothing is running, and `⏎ start` rather than `⏎ attach` where entering means making one.
- One terminal to a session. zmx lets any number of clients attach to one and they mirror each other, keystrokes and all, and nothing here ever does that: a session another terminal has open is taken rather than joined, `enter` asking first because taking drops that terminal's client out from under whoever is at it. Escape refuses it too, and leaves outright where that leaves nothing to go back to. The marker `zmx list` writes on the session the terminal is attached to is what says whose client a circle stands for. There is no read-only attach because zmx has none to offer: `zmx attach` takes `--labels` and nothing else, and `zmx detach` takes no session name either, reading it from `ZMX_SESSION`.
- A session row says where the session stands, in the sessions view, and nothing about it the path does not.
- The session a pane last attached is recorded per pane, keyed by the pid of the shell whose loop runs the picker, which is the same process for as long as the pane is open. The picker opens with the cursor on it, escape leaves for it, and a pane that has held nothing falls through to the shared record. Records of panes that are gone are collected on the next write, nothing else running when a pane closes.
- The preview sits beside the list or above it as the terminal's shape allows, or not at all where the list would be left unreadable. A session previews as its own scrollback, at the end of it; a repository as `git log --graph`, any other directory as its listing, which drops the dependency on `lsd`.

### Added

- `zmd daemon`, which reads what is running every second and credits a visit to every directory a session moves to: zmx follows a session's `cd` and `zmx list` says where it went, which is how the filesystem view learns which directories to list first. A daemon's first reading only seeds where sessions stand, so a restart credits nothing. It talks ConnectRPC over a unix socket beside the rest of the state, or wherever `ZMD_DECK_DIR` says, an SSH `RemoteForward` being the case that needs to choose, and refuses a socket path longer than the platform can bind, naming the limit. It also walks the checkouts below `~` and holds the result, so a picker asks rather than walks: 290ms before a single row was drawn, against 3ms. A picker that cannot reach it walks for itself.

### Removed

- `zmx-nav switch`, `zmx-nav pick` and `zmx-nav new`. There is one picker and it is what the command runs.
- `--root` and `ZMX_NAV_ROOT`. Repositories are looked for below `~`.
- The groupings `tab` cycled, and with them the view the picker remembered against the directory it was opened in.
- `ctrl-n`. Nothing chooses a session's name any more, so the key had nothing left to do.
- `ctrl-x`. Entering a session another terminal holds already takes it, after asking; a second key for the same thing without the asking is one path too many.
- The scope, and the keys that widened and narrowed it. The filesystem view stands in a directory instead.

## [0.3.0] - 2026-09-16

### Added

- `zmx-nav switch` holds a pane over one session after another: it attaches, and offers the picker again once that client is gone. zmx's client takes ctrl+\ before the PTY sees it, so the switch is reachable from inside whatever the session is running rather than only from a shell prompt.
- The switcher's picker cycles a fourth grouping, the repositories a session can be started in, and falls back to it where the grouping asked for holds nothing.

## [0.2.0] - 2026-09-16

### Changed

- The picker cycles its grouping with `tab`, and back with `shift-tab`, in place of `ctrl-f`, `ctrl-d` and `ctrl-r`.

## [0.1.0] - 2026-09-16

### Added

- `zmx-nav pick` attaches to a running zmx session, grouped flat, by working directory, or by repository with worktrees under the repository they belong to. `ctrl-f`, `ctrl-d` and `ctrl-r` swap the grouping without leaving the picker.
- `zmx-nav new` starts a session in any repository below `~/src` or in any worktree git records for one, named `org.repo` and `org.repo@label`. A name already running is attached rather than created.
- `--root` and `ZMX_NAV_ROOT` set where repositories are looked for.

[Unreleased]: https://github.com/mikluko/zmd/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/mikluko/zmd/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/mikluko/zmd/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/mikluko/zmd/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/mikluko/zmd/releases/tag/v0.1.0
