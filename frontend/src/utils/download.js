import { resourcesApi } from "@/api";
import { notify } from "@/notify";
import { getters, mutations, state } from "@/store";

// fork: archive format used for every multi-file / folder download
export const DEFAULT_ARCHIVE_FORMAT = "zip";

export default function downloadFiles(items) {
  if (items.length === 0) {
    notify.showError("No files selected");
    return;
  }
  if (typeof items[0] === "number") {
    // map the index to state.req.items
    items = items.map(i => state.req.items.at(i));
  }
  
  // Chunked single-file (large) vs chunked multi-item archive (folder / multi-select)
  const downloadChunkSizeMb = state.user?.fileLoading?.downloadChunkSizeMb || 0
  const sizeThreshold = downloadChunkSizeMb * 1024 * 1024;
  
  const willUseChunkedDownload =
    downloadChunkSizeMb > 0 &&
    items.length === 1 &&
    !items[0].isDir &&
    items[0].size &&
    items[0].size >= sizeThreshold

  const isMultiItemArchive =
    items.length > 1 || (items.length === 1 && items[0].isDir)

  const willUseChunkedArchive =
    downloadChunkSizeMb > 0 && isMultiItemArchive

  const showChunkedProgressFirst =
    willUseChunkedDownload || willUseChunkedArchive

  if (getters.isShare()) {
    // Perform download without opening a new window
    if (getters.isSingleFileSelected()) {
      if (showChunkedProgressFirst) {
        mutations.showPrompt({ name: "download" });
        void startDownload(null, items, state.shareInfo.hash, {
          silentChunkedError: true,
        });
      } else {
        void startDownload(null, items, state.shareInfo.hash);
      }
    } else {
      // fork: always download multiple files as zip, no format prompt
      void startDownload(DEFAULT_ARCHIVE_FORMAT, items, state.shareInfo.hash, {
        silentChunkedError: willUseChunkedArchive,
      });
    }
    return;
  }

  if (getters.isSingleFileSelected()) {
    if (showChunkedProgressFirst) {
      mutations.showPrompt({ name: "download" });
      void startDownload(null, items, "", { silentChunkedError: true });
    } else {
      void startDownload(null, items);
    }
  } else {
    // fork: always download multiple files as zip, no format prompt
    void startDownload(DEFAULT_ARCHIVE_FORMAT, items, "", {
      silentChunkedError: willUseChunkedArchive,
    });
  }
}

async function startDownload(config, files, hash = "", options = {}) {
  try {
    notify.showSuccessToast("Downloading...");
    await resourcesApi.download(config, files, hash);
  } catch (e) {
    if (e?.name === "AbortError" || e?.message?.includes("aborted") || e?.message?.includes("cancelled")) {
      return;
    }
    if (options.silentChunkedError) {
      return;
    }
    notify.showError(`Error downloading: ${e.message || e}`);
  }
}

/** Start the in-app share download as a zip archive (fork: no format prompt). */
export function showShareDownloadPrompt(items) {
  if (items.length === 0) {
    notify.showError("No files selected");
    return;
  }
  if (typeof items[0] === "number") {
    items = items.map((i) => state.req.items.at(i));
  }

  const downloadChunkSizeMb = state.user?.fileLoading?.downloadChunkSizeMb || 0;
  const isMultiItemArchive =
    items.length > 1 || (items.length === 1 && items[0].isDir);
  const willUseChunkedArchive =
    downloadChunkSizeMb > 0 && isMultiItemArchive;

  void startDownload(DEFAULT_ARCHIVE_FORMAT, items, state.shareInfo?.hash || "", {
    silentChunkedError: willUseChunkedArchive,
  });
}
