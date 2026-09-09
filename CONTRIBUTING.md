# Contributing to runthrough

<!-- SPDX-License-Identifier: GPL-3.0-or-later -->

Thanks for your interest. The project is in early design; the most valuable contributions
right now are discussion on the functional specification in `docs/`.

## Developer Certificate of Origin

This project uses the [Developer Certificate of Origin](https://developercertificate.org/)
(DCO) instead of a Contributor License Agreement. You keep the copyright on what you write;
you simply certify that you have the right to submit it under the project's license.

Sign off every commit:

```bash
git commit -s -m "your message"
```

That appends a line to the commit message:

```
Signed-off-by: Your Name <your.email@example.com>
```

Commits without a sign-off cannot be merged.

## License of contributions

Contributions are licensed under **GPL-3.0-or-later**, the same terms as the project.
There is no copyright assignment: because of that, the project cannot be relicensed
without the agreement of every contributor. That is deliberate.

Add an SPDX header to every new source file:

```go
// SPDX-License-Identifier: GPL-3.0-or-later
```

## Commit message convention

Conventional Commits in the title (so changelog and semver can be automated) plus a
**What / Why** body, so the log still makes sense months later.

```
<type>(<scope>): <imperative summary>            # title, lowercase, no period, ≤72

What: <what changes technically>                 # one line, target ≤100
Why:  <why it is done>                           # one line, no hard cap

<optional context paragraph, wrapped at ~72>

<optional footer: Tags: / Refs: #<issue> / BREAKING CHANGE: <description>>
```

**Types:** `feat`, `fix`, `docs`, `refactor`, `test`, `perf`, `build`, `ci`, `chore`,
`style`, `revert`.

**Scope** — one per commit, from a controlled but soft vocabulary. Reuse an existing scope
before inventing one:

- Structure: `repo`, `docs`, `spec`, `ci`, `build`
- Components: `cli`, `mcp`, `core`
- Modules: `catalog`, `worktree`, `compose`, `infra`, `data`, `probe`, `doctor`, `report`

**Tags** (cross-cutting, for search): `breaking`, `security`, `performance`, `ux`,
`migration`, `dependencies`, `tech-debt`.

Rules:

- `What` and `Why` are **required** for `feat`, `fix`, `refactor` and `perf`; optional for
  trivial `docs`, `chore`, `style` and `test`.
- `What` and `Why` must say **different** things — neither repeats the title nor the other.
- Breaking changes add `!` after the type/scope and a `BREAKING CHANGE:` footer.
- **No AI attribution, secrets or build artifacts in commits.**

Enable the template once per clone:

```bash
git config commit.template .gitmessage
```

## Ground rules

- One concern per pull request.
- Public behaviour changes come with documentation in the same pull request.
- No secrets in code, fixtures, tests or logs — ever.
- Discuss design changes to the specification before implementing them.
