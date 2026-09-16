# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/2.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- One picker of two lists in place of `pick` and `new`: the directories a session can be in, and the sessions there are, with `tab` between them. A directory carries one circle per session in it, filled where a client is attached and hollow where none is, counted by the directory a session runs in rather than by its name.
- `enter` on a directory attaches the session there, starts one where there is none, and opens that directory's sessions where there are several. `alt-enter` starts another under a label typed at the prompt, named `org.repo+label`. There is no `ctrl-enter`: a terminal sends `enter`'s own byte for it.
- `delete` kills the session under the cursor after asking, and `ctrl-delete` without asking. Backspace kills nothing: it is how a query is corrected.
- The cursor starts on the directory the terminal was opened in, closest match winning, so a worktree answers with itself rather than with its repository.
- The query sits at the top, and the preview beside the list or below it as the terminal's shape allows, or not at all where the list would be left unreadable.
- fzf is a library rather than a command, so a mode is a loop here and each list is free to show its own columns.

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

[Unreleased]: https://github.com/mikluko/zmx-nav/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/mikluko/zmx-nav/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/mikluko/zmx-nav/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/mikluko/zmx-nav/releases/tag/v0.1.0
