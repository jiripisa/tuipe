---
name: release
description: Use when the user wants to release, publish or ship a new version of tuipe ("udělej release", "vydej novou verzi", "make a release").
---

# Releasing tuipe

A release is a pushed tag `vX.Y.Z` on `main`. Everything after the push runs
in GitHub Actions (`.github/workflows/release.yml`): the tests on Linux, macOS
and Windows, GoReleaser (archives for three systems and two CPUs,
`checksums.txt`, release notes from the commits), then `install.sh` and
`install.ps1` install the release and run it. No file in the repo carries the
version; the tag is the version. Every step below has a check; a step is not
done until its check passed.

**Pushing needs the user's explicit OK** (global rule). Before step 4 ask one
bold question that names the tag push (`vX.Y.Z`, which publishes the release)
and, for a release without `-` in the tag, the catalog update of step 6 (a
push to `jiripisa/jiripisa.github.io` or a rebuild of it), unless the user has
already said yes to exactly that in the conversation.

## 1. Prepare

1. On `main`, `git status` clean, up to date with `origin/main`
   (`git fetch` first). A release comes only from `main`; work on another
   branch is merged first.
2. CI is green on that commit:
   `gh run list --workflow CI --branch main --limit 1` shows `success` for
   `git rev-parse --short HEAD`.
3. `go vet ./...` and `go test ./...` pass locally (redirect the output to a
   file, read the summary).

## 2. Pick the version

Count from the last tag (`git describe --tags --abbrev=0`) and read
`git log --oneline <last tag>..HEAD`: patch for fixes, minor for new features,
major only when the user says so. A test release gets a `-rc.N` suffix;
GoReleaser marks it a prerelease, so `releases/latest`, the installers and
`tuipe upgrade` keep the previous version. Say which version and why.

The release notes are the commit subjects since the last tag, without
`docs:` and `test:` (`.goreleaser.yaml`). Show the user that list.

## 3. Tag

An annotated tag, as the earlier ones:

```bash
git tag -a vX.Y.Z -m "tuipe vX.Y.Z"
```

## 4. Publish

`git push origin vX.Y.Z`. Then wait for the Release workflow:

```bash
sleep 5; gh run list --workflow Release --limit 1
gh run watch <id> --exit-status
```

Check: the jobs `test`, `release` and (without `-`) `installed` all passed.
A failed `test` job publishes nothing: fix on `main`, delete the tag locally
and on GitHub (`git push origin :refs/tags/vX.Y.Z`, after asking), tag again.

## 5. Live checks

1. `gh release view vX.Y.Z --json isPrerelease,assets --jq '.isPrerelease, (.assets[].name)'`
   lists six archives (`tuipe_X.Y.Z_{darwin,linux,windows}_{amd64,arm64}`,
   `.tar.gz`, Windows `.zip`) and `checksums.txt`.
2. Without `-`: `gh api repos/jiripisa/tuipe/releases/latest --jq .tag_name`
   says `vX.Y.Z`.

## 6. Project catalog

Only for a release without `-`, after the live checks passed: run the global
`update-catalog` skill for vX.Y.Z. It puts tuipe into the right section of the
catalog at https://jiripisa.github.io/, updates its summary when this release
changed what tuipe does, and publishes the catalog so the new download shows
there.

## Report

Tell the user: the release URL, what was pushed, the results of every check,
and the version choice and why.
