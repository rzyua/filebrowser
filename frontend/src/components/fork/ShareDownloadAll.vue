<template>
  <button
    v-if="visible"
    type="button"
    class="share-download-all"
    :aria-label="label"
    :title="label"
    :disabled="disabled"
    @click="downloadAll"
  >
    <i class="material-symbols">download</i>
    <span class="share-download-all__label">{{ label }}</span>
  </button>
</template>

<script>
// fork: prominent "Download all" button shown in the header of share listings,
// so visitors can grab the whole share even when the share's sidebar is disabled.
import { state, getters } from "@/store";
import { showShareDownloadPrompt } from "@/utils/download.js";

// Kept local to this component so the fork does not touch upstream locale files.
const LABELS = new Map([
  ["en", "Download all"],
  ["pl", "Pobierz wszystko"],
  ["de", "Alle herunterladen"],
  ["fr", "Tout télécharger"],
  ["es", "Descargar todo"],
]);

export default {
  name: "ShareDownloadAll",
  computed: {
    visible() {
      if (!getters.isShare() || getters.currentView() !== "listingView") {
        return false;
      }
      const info = state.shareInfo || {};
      if (info.shareType === "upload" || info.disableDownload) {
        return false;
      }
      return (state.req?.items?.length || 0) > 0;
    },
    disabled() {
      return getters.currentPromptName() !== "";
    },
    label() {
      const locale = String(this.$i18n?.locale || "en").toLowerCase();
      return LABELS.get(locale) || LABELS.get(locale.split(/[-_]/)[0]) || LABELS.get("en");
    },
  },
  methods: {
    downloadAll() {
      const req = state.req || {};
      showShareDownloadPrompt([
        {
          path: req.path || "/",
          name: req.name || state.shareInfo?.title || "download",
          type: "directory",
          size: req.size,
          isDir: true,
        },
      ]);
    },
  },
};
</script>

<style scoped>
.share-download-all {
  display: inline-flex;
  align-items: center;
  gap: 0.4em;
  flex: 0 0 auto;
  height: 2.5em;
  padding: 0 1em;
  margin: 0 0.25em;
  border: none;
  border-radius: 1.25em;
  background: var(--primaryColor);
  color: #fff;
  font-size: 0.95em;
  font-weight: 500;
  cursor: pointer;
  white-space: nowrap;
  transition: filter 0.15s ease;
}

.share-download-all:hover:not(:disabled) {
  filter: brightness(1.1);
}

.share-download-all:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.share-download-all .material-symbols {
  font-size: 1.3em;
}

@media (max-width: 480px) {
  .share-download-all {
    padding: 0 0.75em;
  }
  .share-download-all__label {
    display: none;
  }
}
</style>
