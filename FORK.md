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
| **Sidebar hidden by default** on new shares (the "Disable sidebar" toggle starts on; existing shares keep their setting). | 2 lines in `frontend/src/components/prompts/Share.vue` | Upstream adds a configurable share default |
| **Polish translation** rewritten by hand (context-aware, consistent terminology, placeholders fixed). | `frontend/src/i18n/pl.json` | Never; re-check new keys after each upstream release |
| **Fork tooling**: this file, `fork-*` workflows, Renovate config. | `FORK.md`, `.github/workflows/fork-*.yaml`, `.github/renovate.json` | - |

Rules that keep upstream syncs cheap:

- Add new files rather than editing upstream ones; keep edits to upstream files to a few lines, marked `// fork:`.
- Never rename the Go module or edit upstream workflow files.
- New UI strings live inside the fork component (see `ShareDownloadAll.vue`), not in the 26 locale files.
- Don't bump Go or npm dependencies in the fork; upstream does that.

## Branches

| Branch | Contents |
| --- | --- |
| `custom` (default) | Latest upstream `v*-stable` tag + the `fork:` commits. Rewritten on every upstream release. |
| `snapshot/<tag>` | Frozen copy of `custom` for each upstream release; rollback points. |
| `sync/<tag>` | Temporary branch made by the sync workflow while a new release is tested. |
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
2. Runs frontend lint/tests, the translation check, Go tests and an image build with smoke test.
3. Waits for approval on the `fork-promote` environment, then moves `custom`, keeps `snapshot/<old-tag>` and `snapshot/<new-tag>`, and publishes the image.
4. On a conflict, opens an issue naming the failing `fork:` commit and the conflicting files.
5. Weekly, dry-runs the rebase onto upstream `main` (the next beta) and opens a heads-up issue if a future release would conflict.

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
