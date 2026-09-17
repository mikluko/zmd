# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/2.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Renamed to `zmd`, from `zmx-nav`. The old name described navigating between lists, which is neither what the tool does nor what it is for; this one is zmx's deck, the place you stand to see the whole of a thing and take it somewhere. The module path and the command both move, and `go install github.com/mikluko/zmx-nav` installs nothing after this.
- One picker, and a stack of rungs inside it: the sessions running, the repositories one can be started in, and a repository's checkouts. `right` pushes, `left` pops back to wherever it was pushed from, `enter` is right's alias, and `backspace` pops with `left`. All three correct the query first where there is one, since a picker that cannot correct what was typed is worse than one that cannot walk.
- Sessions are the ground floor and where the picker opens. They are what a switch is between and what a scan is over: an agent wanting attention and a shell left running are both facts about sessions, and looking for either in a list of repositories means reading a list twenty times longer and then descending to learn which session it was. The repositories above are how a session is started somewhere new, which is the rarer thing and the only thing they are for; a checkout that already holds sessions pushes straight back down to them, filtered to itself, rather than listing them a second way. `ctrl-g` is the way up from the ground floor.
- Sessions are named `word-NN` — `harbor-41`, `sage-03` — and the name says nothing. What a session is for is metadata, `zmx set <name> title=…`, which rides on the record `zmx list` already writes; the handle is the fallback and what a session goes by where nothing has labelled it.
- A place's name is what git says it is, `owner/name` off the checkout's remote, read out of `.git/config` with go-git's config parser rather than by opening the repository: go-git refuses one whose `core.repositoryformatversion` carries an extension it does not implement, and `worktreeconfig` is one git sets on exactly the repositories worktrees have been cut from. The path is where a checkout happens to sit and says nothing reliable about what it holds. A worktree goes by its repository and what is left of its own basename once the repository's name is taken off the front. 96 of 114 checkouts answer in three milliseconds, beside the walk.
- A status column three cells wide ahead of every name, so a name starts in the same place on every line. The first says what git makes of the directory, bright for a repository and dim for a worktree cut from one; the second says whether the session has a client, or on the repositories rung how many are alive at or below the row; the third is held for what the agent in a session is doing and is blank until something sets it. Being fixed is what lets the column carry colour: `text/tabwriter` counts an escape byte as a character, so the column is written past the alignment rather than through it.
- The hint line offers only the keys that would do something where the cursor stands: no `⌦ kill` where nothing is running, `⏎ start` rather than `⏎ attach` where entering means making one, and no `← back` on the ground floor. fzf rewrites it per row through the same `focus` transform that sets the preview's follow mode, since between turns of the loop nothing of this program is running to be asked. A test reads the source and fails where a key the loop acts on is not one the picker is told to expect, a key being dead however plainly the hint line offers it.
- One terminal to a session. zmx lets any number of clients attach to one and they mirror each other, keystrokes and all, and nothing here ever does that: a session another terminal has open is taken rather than joined, `enter` asking first because taking drops that terminal's client out from under whoever is at it. Escape refuses it too, and leaves outright where that leaves nothing to go back to. The marker `zmx list` writes on the session the terminal is attached to is what says whose client a circle stands for. There is no read-only attach because zmx has none to offer: `zmx attach` takes `--labels` and nothing else, and `zmx detach` takes no session name either, reading it from `ZMX_SESSION`.
- The session a pane last attached is recorded per pane, keyed by the pid of the shell whose loop runs the picker, which is the same process for as long as the pane is open. The picker opens with the cursor on it, escape leaves for it, and a pane that has held nothing falls through to the shared record. Records of panes that are gone are collected on the next write, nothing else running when a pane closes.
- The preview sits beside the list or below it as the terminal's shape allows, or not at all where the list would be left unreadable. A session previews as its own scrollback, at the end of it; a place as `git log --graph`, which drops the dependency on `lsd`.

### Removed

- `zmx-nav switch`, `zmx-nav pick` and `zmx-nav new`. There is one picker and it is what the command runs.
- `--root` and `ZMX_NAV_ROOT`. Repositories are looked for below `~`.
- The groupings `tab` cycled, and with them the view the picker remembered against the directory it was opened in: one list is not a thing there is anything to remember about.
- `ctrl-n`. Nothing chooses a session's name any more, so the key had nothing left to do.
- `ctrl-x`. Entering a session another terminal holds already takes it, after asking; a second key for the same thing without the asking is one path too many.
- The scope, and the keys that widened and narrowed it. The stack is what walks now, and a rung is a place to stand rather than a filter over one list.

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

[Unreleased]: https://github.com/mikluko/zmd/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/mikluko/zmd/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/mikluko/zmd/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/mikluko/zmd/releases/tag/v0.1.0
