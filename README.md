# runthrough

<!-- SPDX-License-Identifier: GPL-3.0-or-later -->

**Run your whole stack the way it will actually run — before it ships.**

`runthrough` brings up a full service ecosystem locally, in containers, built straight from
your git worktrees. It is language-agnostic: Go microservices, a NestJS BFF, an Angular
front end and a PHP monolith are all just services with a runtime declared in a catalog.

A *run-through* is the rehearsal where every piece runs together, in order, before opening
night. That is what this tool is for.

> **Status: early design.** No usable release yet. The functional specification lives in
> [`docs/00-spec.md`](docs/00-spec.md) (Spanish; an English translation is planned).

## Why

Local development environments drift. Services get run by hand on different ports, against
different databases, from whichever branch happened to be checked out. When something
breaks, nobody can say whether it was the change or the environment.

`runthrough` makes the environment an instrument you can trust:

- **Worktree-native.** Each service is built from a specific git worktree, so you can pin
  one service to a feature branch while the rest stay on the release branch.
- **Traceable.** Images are labelled with repo, branch and commit; `status` tells you what
  is actually running.
- **Reproducible.** Snapshot and restore the database so two runs start from the same
  point and a before/after comparison means something.
- **Switchable infrastructure.** Point the stack at containerized infra, at services on
  your own machine, or at shared remote infra — per profile, or per service.
- **A gateway and a probe.** One entry point that mirrors the real routing, plus a host
  port per service so you can test one service in isolation.
- **Usable by agents.** The same operations are exposed as an MCP server, with structured
  JSON output and destructive commands blocked against shared infrastructure.

It is the sibling of [grove](https://github.com/neytor/grove): grove manages the *code*
(one worktree per branch), `runthrough` manages its *execution*.

## Documentation

- [`docs/00-spec.md`](docs/00-spec.md) — functional specification: capability catalog,
  architecture, CLI and MCP surface, roadmap.

## License

`runthrough` is free software: you can redistribute it and/or modify it under the terms of
the **GNU General Public License** as published by the Free Software Foundation, either
version 3 of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY;
without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
See the GNU General Public License for more details. You should have received a copy of the
license along with this program; if not, see <https://www.gnu.org/licenses/>.

**The license covers the tool, not your data.** Catalogs, compose files, environment files
and configuration you write for your own stack are inputs to the program, not derivative
works of it.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Contributions are accepted under the DCO: sign your
commits with `git commit -s`.
