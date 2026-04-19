---
description: Bump VERSION, verify build, commit, tag, and push a new release
argument-hint: "[major|minor|patch]  (default: minor)"
---

Publish a new release of logsee by wrapping `scripts/publish.sh`.

The script itself does the version bump, tag existence check, commit, tag,
and push. This command only adds the clean-tree precondition and surfaces
results back to the user.

## Arguments

`$ARGUMENTS` — one of `major`, `minor`, `patch`, or empty. Empty means
`minor`. Anything else: stop and ask the user to correct.

## Steps

1. **Decide the bump.** If `$ARGUMENTS` is empty → `BUMP=minor`. Else
   validate it is exactly one of `major`/`minor`/`patch` (trim whitespace).
   Reject anything else with a short error — do not proceed.

2. **Clean working tree.** Run `git status --porcelain`. If the output is
   non-empty (any staged, unstaged, or untracked file), stop and tell the
   user which files are dirty. The release commit must start from a clean
   HEAD — otherwise `scripts/publish.sh` would sweep unrelated changes
   into the release commit via `git add -A`.

3. **Read current VERSION and compute target tag.** Read `VERSION`, compute
   the next semver according to `BUMP`, and check that the tag does NOT
   already exist either locally or on `origin`:

   ```bash
   git rev-parse "refs/tags/v${NEW_VER}" >/dev/null 2>&1 && \
     echo "tag v${NEW_VER} already exists" && exit 1
   git ls-remote --tags origin "refs/tags/v${NEW_VER}" | grep -q . && \
     echo "remote tag v${NEW_VER} already exists" && exit 1
   ```

   Report the intended new version to the user before running the script
   so they can cancel.

4. **Preflight: fmt-check, vet, test, build.** Run `make publish-verify`.
   If it fails, stop and surface the failing target — do not continue.

5. **Run the publish script.** Invoke `./scripts/publish.sh <BUMP>`. The
   script repeats step 3's tag existence check, bumps VERSION, commits
   `chore: release vX.Y.Z`, creates an annotated tag `vX.Y.Z`, and pushes
   the current branch plus the tag to `origin`.

6. **Report.** On success, print the new version, the pushed branch, the
   tag, and the latest commit SHA. On failure mid-script, describe the
   state (VERSION bumped or not, commit created or not, tag created or
   not, push completed or not) so the user can decide how to recover.

## Guardrails

- Never pass `--no-verify` or `--no-gpg-sign` to git.
- Never force-push.
- If the user is on `main`/`master`, confirm with them before pushing if
  the remote branch has diverged (i.e. `git fetch && git status` shows
  `behind`).
- Stop if `origin` is not configured — this repo expects pushes to
  `origin`.
