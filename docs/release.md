# Releasing Pass Quest

Pushing a version tag builds a release; publishing it is the owner's decision. The workflow is
[`.github/workflows/release.yml`](../.github/workflows/release.yml).

## Steps

1. Set the new version in `mygo.json` and in `update.Version` (`internal/update/update.go`). A Go
   test fails when the two differ. Merge that change to `main` through a pull request, the same way
   as any other change.
2. Tag the merged commit on `main` and push the tag:

   ```sh
   git switch main && git pull --ff-only
   git tag v0.16.0
   git push origin v0.16.0
   ```

3. The workflow runs in this order:
   - It checks that the tag equals `v` plus `mygo.json`'s version, and stops otherwise.
   - It runs the full gate (`gate.yml`: every group on Windows, macOS and Linux) on the tagged commit.
   - It collects the three packages that the gate's own `package` jobs built, installed and smoke-tested.
   - It writes `SHA256SUMS.txt`.
   - It creates a **draft** release. The notes are the header in
     [`.github/release-notes.md`](../.github/release-notes.md) followed by GitHub's generated list of
     merged pull requests.
4. Review the draft on GitHub (files, notes), then press **Publish release**. Until then nobody sees
   it, and the app's update check does not find it. The check reads `/releases/latest`, which excludes
   drafts.
5. After publishing, point `PREVIOUS_RELEASE_COMMIT` in `harness/gates/package-smoke/upgrade.ts` at
   the new tag's full commit id. The next change's forward-upgrade check then starts from the
   release learners actually have. Merge that change like any other.

## If something fails

- **Tag and version differ.** Delete the tag (`git push origin :refs/tags/v0.16.0` and
  `git tag -d v0.16.0`), fix `mygo.json` on `main`, and tag again.
- **A gate job fails.** No release is created. Fix the cause on `main`, then move the tag to the
  fixed commit (delete it and tag again) or use the next version.
- **A release for the tag already exists.** The workflow fails rather than replacing it. Delete the
  draft on GitHub first if it should be rebuilt.

## What the workflow does not do

- It does not sign the installers (owner decision in goal 0013: unsigned packages).
- It does not change the version for you.
- It does not publish without the owner (Q-0000-06).
