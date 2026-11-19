# Maintainer notes

Why releases and CI work the way they do. The step-by-step release procedure is in [release.md](release.md); the
checklist after tagging is in [CONTRIBUTING.md → Releases](../CONTRIBUTING.md#releases-maintainer-only).

## The principle: a safe system makes contributions easy

Outside pull requests should be easy to accept. That is only possible when a pull request cannot reach anything
that matters: signing keys, the release, or users' machines. So the safety lives in the system, not in trusting each
contributor:

- CI (`ci.yml`) runs on `pull_request`, never `pull_request_target`, with `permissions: contents: read`. A fork's
  run gets no secrets and a read-only token, so its code cannot sign, publish or push.
- Workflow runs from external contributors wait for maintainer approval. Read the diff before approving, especially
  changes to `.github/`, `scripts/`, `Package.swift`, `go.mod`, `Makefile` or `packaging/` (the CODEOWNERS
  security-sensitive paths; the `security-sensitive` label marks such PRs).
- The default workflow token is read-only. Release jobs add only what they need: `id-token`/`attestations` for
  provenance, `contents: write` only in the job that creates the GitHub Release.
- Every Action is pinned to a full commit SHA with the version in a comment. Dependabot proposes updates; no new
  third-party Actions.
- `main` requires the `test` check and allows no force pushes. Tags `v*` are protected by the `protect-release-tags`
  ruleset (creation, update and deletion are limited to admins), so only the maintainer can start a release.

## Releases: notarized or nothing

- Every macOS release is Developer ID signed, notarized by Apple and stapled. If the signing secrets are missing, the
  release fails; it never falls back to an ad-hoc build.
- Releases run in two phases. Phase 1 (tag push) builds, signs and submits to Apple without waiting, and keeps the
  exact submitted zip and its submission id as an artifact for 90 days. Phase 2 (manual, `-f run_id=<phase-1 run>`)
  runs after Apple says `Accepted`: it staples that same app, verifies it, attests provenance and publishes.
- Apple can take hours for a new account. Do not re-run phase 1 to wait: each run builds and submits a new app, and
  the late `Accepted` then belongs to an app that no longer exists. Check the submission from a Mac and run phase 2
  once it is accepted ([release.md](release.md#맥에서-제출-id-조회하기-check-a-submission-from-a-mac)).
- Release notes in `docs/release-notes/vX.Y.Z.md` become the release body; the tap and AUR are bumped afterwards by
  scripts that verify checksums and provenance first.

## In-app updates verify before replacing

The macOS app replaces itself only after the downloaded zip passes every check (`UpdateInstaller` in Core,
`UpdateController` in the app): the SHA-256 from the release's `.sha256` file, a zip listing with nothing outside
`Bulchimbeon.app/` (no absolute paths, `..` or symlinks), then a valid code signature with the same signing
identifier and the same **team ID** as the running app, hardened runtime, and exactly the offered version. A build
signed by anyone else, or unsigned, is refused. Keep these checks when changing the updater, and keep the signing
team stable: a team change makes every zip-installed copy refuse the update once (as happened at 0.12.6).

## Commits and identity

Commit under an identity linked to your GitHub account (a noreply address is fine), so authorship is clear; never
commit secrets, `.env` files or local settings. Contributors disclose
material AI assistance in the pull request template ([CONTRIBUTING.md](../CONTRIBUTING.md#ai-assisted-contributions)).
