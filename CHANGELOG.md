# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/2.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Renamed to `zmx-deck`, from `zmx-nav`. The old name described navigating between lists, which is neither what the tool does nor what it is for; a deck is where you stand to see the whole of a thing and take it somewhere. The module path and the command both move; `go install github.com/mikluko/zmx-nav` installs nothing after this.
- One picker of one list in place of `pick` and `new`: the sessions there are, then the repositories a session can be started in. Typing is the navigation, narrowing both kinds at once, and the kind of the row it leaves under the cursor is what decides what a key does to it.
- `enter` attaches, and starts a session in a repository that has none. Where a repository has several it narrows the list to them rather than guessing between them, which is the same query mechanism the rest of the picker is made of. `alt-enter` starts another under a label typed at the prompt. There is no `ctrl-enter`: a terminal sends `enter`'s own byte for it.
- A query matching nothing names a session to start where the pane stands, and `ctrl-n` forces that even where a row is highlighted.
- `tab` marks, and `delete` kills what is marked or what is under the cursor, after asking; `ctrl-delete` without asking. Backspace kills nothing: it is how a query is corrected. The session the picker is running inside is killed last, since killing it takes the picker down with it.
- A repository carries one circle per session in it, filled where a client is attached and hollow where none is, counted by the directory a session runs in rather than by its name.
- The cursor starts on the row closest to the directory the terminal was opened in, so a worktree answers with itself rather than with its repository, and a directory that has a session answers with the session rather than with the offer to start a second.
- A repository previews as `git log --graph` rather than as a directory listing, which drops the dependency on `lsd`. The preview sits beside the list or below it as the terminal's shape allows, or not at all where the list would be left unreadable.
- Sessions are named after the path, dotted, taken relative to `~`: a repository name repeats across orgs, and a bare name would send the second one into the first one's session.

### Removed

- `zmx-nav switch`, `zmx-nav pick` and `zmx-nav new`. There is one picker and it is what the command runs.
- `--root` and `ZMX_NAV_ROOT`. Repositories are looked for below `~`.
- The groupings `tab` cycled, and with them the view the picker remembered against the directory it was opened in: one list is not a thing there is anything to remember about.

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
- `zmx-nav new` starts a session in any repository below `~/Forge` or in any worktree git records for one, named `org.repo` and `org.repo@label`. A name already running is attached rather than created.
- `--root` and `ZMX_NAV_ROOT` set where repositories are looked for.

[Unreleased]: https://github.com/mikluko/zmx-deck/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/mikluko/zmx-deck/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/mikluko/zmx-deck/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/mikluko/zmx-deck/releases/tag/v0.1.0
