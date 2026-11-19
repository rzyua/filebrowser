# rzyua/filebrowser fork

A personal fork of [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser)
that follows upstream **stable** releases and adds a few small changes on top.
The image is published for **linux/arm64** at `ghcr.io/rzyua/filebrowser`.

## Modifications

Each modification is one commit whose subject starts with `fork:`. List them with:

```sh
git log --oneline $(git tag --merged custom -l 'v*-stable' --sort=-v:refname | head -n1)..custom
```

| Change | Files | Drop it when |
| --- | --- | --- |
| **Download-all button** in the share header, so shares with the sidebar disabled can still be downloaded in one click. Hidden for upload-only shares and shares with downloads disabled. | `frontend/src/components/fork/ShareDownloadAll.vue`, 3 lines in `frontend/src/views/bars/Default.vue` | Upstream adds an equivalent button |
| **Zip by default**: multi-file and folder downloads skip the zip/tar.gz prompt. | `frontend/src/utils/download.js`, `frontend/src/api/resources.js` | Upstream adds a "default archive format" setting |
| **Sidebar hidden by default** on new shares (the "Disable sidebar" toggle starts on; existing shares keep their setting). | 2 lines in `frontend/src/components/prompts/Share.vue` | Upstream `main` (2.1 beta line) already has admin-configurable share defaults. When that reaches a stable release this commit will conflict; drop it (`git rebase` → `drop`) and set the default in upstream's settings instead |
| **Polish translation** rewritten by hand, kept as an overlay merged over upstream's `pl.json` at load time, so upstream's translation updates never conflict. New upstream strings show upstream's text until added to the overlay. | `frontend/src/i18n/fork/pl.json`, `frontend/src/i18n/fork/overlay.ts`, 1 line in `frontend/src/main.ts`, `frontend/scripts/fork-i18n-check.js` | Never; add new strings after each upstream release (see below) |
| **Fork tooling**: this file, `fork-*` workflows, Renovate config. | `FORK.md`, `.github/workflows/fork-*.yaml`, `.github/renovate.json` | - |

Rules that keep upstream syncs cheap:

- Add new files rather than editing upstream ones; keep edits to upstream files to a few lines, marked `// fork:`.
- Never rename the Go module or edit upstream workflow files.
- New UI strings live inside the fork component (see `ShareDownloadAll.vue`), not in the 26 locale files.
- Translation fixes go in `frontend/src/i18n/fork/<locale>.json`, never in upstream's locale files.
- Don't bump Go or npm dependencies in the fork; upstream does that.

## Branches

| Branch | Contents |
| --- | --- |
| `custom` (default) | Latest upstream `v*-stable` tag + the `fork:` commits. Rewritten on every upstream release. |
| `snapshot/<tag>` | Frozen copy of `custom` for each upstream release; rollback points. |
| `sync/<tag>` | Temporary branch made by the sync workflow while a new release is tested. |
| `sync-test/<branch>` | Same, for test runs against a scratch branch. |
| `main` | Upstream's `main`, untouched. |

## Image

Tags on `ghcr.io/rzyua/filebrowser`:

- `2.0.2-stable-rz.4`: upstream version + number of fork commits on top. Pin this one on the server.
- `sha-<commit>`: exact build.
- `stable`, `latest`: newest fork build.

Built by `.github/workflows/fork-image.yaml` from upstream's own `_docker/Dockerfile` (the full image, with ffmpeg and MuPDF) on GitHub's native arm64 runners. It runs on every push to `custom` (build, smoke test, push), on pushes to `claude/**` and `feat/**` (build and smoke test only), and weekly to pick up Alpine security fixes.

Build locally:

```sh
docker buildx build --platform linux/arm64 -f _docker/Dockerfile \
  --build-arg VERSION=dev-rz --build-arg REVISION=$(git rev-parse --short HEAD) \
  -t filebrowser:dev --load .
```

## Staying up to date

The **fork upstream sync** workflow (`.github/workflows/fork-sync.yaml`):

1. Daily, checks for a newer upstream `v*-stable` tag and rebases the `fork:` commits onto it as `sync/<tag>`. Conflicts it has seen before are resolved automatically (`git rerere`).
2. Runs frontend lint/tests, the translation check, Go tests and an image build with smoke test. The job summary lists new upstream strings that are missing from the Polish overlay.
3. Waits for approval on the `fork-promote` environment, then moves `custom`, keeps `snapshot/<old-tag>` and `snapshot/<new-tag>`, and publishes the image.
4. On a conflict, opens an issue naming the failing `fork:` commit and the conflicting files (Issues must be enabled under Settings > General > Features; forks start with them off). The same report is always in the run summary.
5. Weekly, dry-runs the rebase onto upstream `main` (the next beta) and opens a heads-up issue if a future release would conflict.

Testing the sync without touching `custom`: push a scratch branch whose `fork:` commits sit on an older stable tag, then run the workflow with `branch` set to it. It rebases, tests and builds, waits for approval, and moves only that branch; snapshots and publishing happen only for `custom`.

```sh
git checkout -B test/sync custom
git rebase --onto v2.0.1-stable v2.0.2-stable
git push -f origin test/sync
# Actions > fork upstream sync > Run workflow: branch = test/sync, tag = v2.0.2-stable
```

Translating new upstream strings after an upgrade:

```sh
cd frontend && node scripts/fork-i18n-check.js   # lists keys to translate
# add them to src/i18n/fork/pl.json, commit with a fork: prefix (or fixup into the Polish commit)
```

Rolling back: point the server at the previous image tag, or reset `custom` to a snapshot:

```sh
git push --force-with-lease origin snapshot/<old-tag>:custom   # republishes that build
```

Resolving a conflict by hand:

```sh
git remote add upstream https://github.com/gtsteffaniak/filebrowser   # once
git config rerere.enabled true                                         # once
git fetch upstream --tags
git checkout -B sync/<new-tag> origin/custom
git rebase --onto <new-tag> <old-tag>
# fix, git add, git rebase --continue, then test
git push --force-with-lease origin sync/<new-tag>:custom
```

## One-time GitHub setup

1. **Actions**: enable workflows in the Actions tab, then disable upstream's `main release`, `tag update`, `PR Request`, `Create Dev Version`, `Promote Dev to Beta` and `Update Build Images` (they publish to upstream's registries or manage upstream's release branches). Leave `regular tests`.
2. **Default branch**: set to `custom`.
3. **Environment**: create `fork-promote` under Settings > Environments and add yourself as a required reviewer. Without a reviewer, upgrades publish automatically.
4. **Package**: after the first push, set the `filebrowser` package to public under the profile's Packages tab.
5. **Renovate** (optional): install the Renovate GitHub app on this repository; it only touches `fork-*.yaml`.
