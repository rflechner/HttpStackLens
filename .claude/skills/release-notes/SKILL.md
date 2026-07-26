---
name: release-notes
description: >-
  Draft or update the repository's RELEASE.md changelog by summarizing the git
  commits added since the last documented version. Use this skill whenever the
  user wants to write release notes, update the changelog, prepare a release,
  document what changed since the last tag, or asks "what's new in this version"
  — even if they don't say "RELEASE.md" by name. It groups commits into features
  and bug fixes and prepends a new version section.
---

# Release notes

Maintain `RELEASE.md` at the repo root: a human-readable, SemVer-ordered log of
what shipped in each version. The audience is someone deciding whether to
upgrade — they care about **new features** and **bug fixes**, not the internal
churn. So this is a *summary*, not a `git log` dump.

## How the range is decided

The whole point is to document only what's genuinely new. That means finding the
gap between the latest tag and the most recent version already written in
`RELEASE.md`, then covering the commits in between.

Run the bundled script from the repo root — it works this out for you:

```bash
.claude/skills/release-notes/scripts/pending_commits.sh
```

It prints `current_tag`, `documented` (the topmost `## vX.Y.Z` heading in
`RELEASE.md`), the `range` it derived, and the non-merge commits in that range.
For example, if the latest tag is `v1.2.0` and `RELEASE.md` already leads with
`## v1.1.0`, it lists the commits in `v1.1.0..v1.2.0` — precisely the ones the
new `v1.2.0` section should describe.

Handle what it reports:
- **`nothing to add`** → `RELEASE.md` already documents the latest tag. Tell the
  user; don't invent a section.
- **`documented=<none>`** → no `RELEASE.md` yet (or it has no version heading).
  You're bootstrapping the first entry, which covers the whole history up to the
  current tag. Summarize it thematically rather than transcribing every commit.
- **commits listed** → proceed to write the new section.

If the user wants to document commits **before** tagging (an in-progress
release), there may be commits after the latest tag. In that case gather them
with `git log --no-merges --reverse --pretty=format:'%h%x09%s' <latest-tag>..HEAD`
and title the section `## Unreleased` until a tag exists.

## Turning commits into notes

Commits here follow conventional-commit prefixes (`feat:`, `fix:`, `chore:`,
`refactor:`, `docs:`, `devops:`, `test:`, …). Use the prefix to sort each commit,
then rewrite its subject into a clear, user-facing line — drop the prefix, fix
terse phrasing, and merge several commits that add up to one feature into a
single bullet. A reader should understand the change without knowing the code.

Sort into sections, and **only include sections that have content**:

- `feat:` → **Features**
- `fix:` → **Bug fixes**
- `perf:` → fold into **Features** as a performance line, or its own
  **Performance** section if there are several
- `refactor:`, `chore:`, `docs:`, `devops:`, `build:`, `ci:` → condense into a
  short **Maintenance** section (one line per meaningful theme, not per commit)
- `test:`, `style:`, formatting-only, and other pure internal noise → omit
  entirely; they don't help a reader deciding whether to upgrade

Keep the short hash on each bullet so a curious reader can trace it back. If a
change landed via a pull request whose number you can determine (from a merge
commit or `gh pr list`), prefer citing `(#NN)` instead of the hash.

## RELEASE.md format

New versions go **at the top** (newest first). Use this structure:

```markdown
# Release Notes

All notable changes to this project are documented here, newest first.
Versions follow [Semantic Versioning](https://semver.org).

## vX.Y.Z — YYYY-MM-DD

### Features
- Concise description of the feature (`abc1234`)

### Bug fixes
- What was broken and is now fixed (`def5678`)

### Maintenance
- Condensed theme, e.g. "CI: build + release workflow for Windows and macOS"
```

Rules that keep the file consistent:
- The version heading is the **tag name verbatim** (e.g. `## v1.2.0`), so the
  script can match it next time. Don't strip or reformat the `v`.
- Use today's date (`YYYY-MM-DD`) unless the user gives the release date.
- When updating an existing file, **prepend** the new section directly under the
  intro — never rewrite or reorder the sections already there.
- Omit empty sections. A release with only fixes shows only **Bug fixes**.

## Workflow

1. Run `pending_commits.sh` and read what it reports.
2. If there's nothing to add, say so and stop.
3. Draft the new version section from the commit list, following the rules above.
4. Show the user the drafted section before writing, so they can adjust wording
   or move a commit between sections — release notes are a communication
   artifact and their judgment on emphasis matters.
5. Prepend it to `RELEASE.md` (creating the file with the intro header if it
   doesn't exist yet).
6. Leave committing/tagging to the user — this repo's maintainer manages version
   control themselves.
