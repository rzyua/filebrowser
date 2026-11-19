// fork: hand-maintained translation overrides.
//
// Files in this folder (e.g. pl.json) are deep-merged over upstream's locale
// files whenever a locale is loaded. Upstream keeps regenerating its own
// locale files, so keeping our translations here means upstream syncs never
// conflict on them. Strings missing here fall back to upstream's text.
import i18n, { availableLocales } from '@/i18n';

type Messages = Record<string, unknown>;

const overlayModules = import.meta.glob<{ default: Messages }>('./*.json', { eager: true });

// Map overlay file name (pl, pt-br, ...) to the internal locale key (pl, ptBR, ...).
const overlays = new Map<string, Messages>();
for (const [key, file] of Object.entries(availableLocales)) {
  const mod = overlayModules[`./${file}.json`];
  if (mod) overlays.set(key, mod.default);
}

function isObject(value: unknown): value is Messages {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

export function mergeMessages(base: Messages, overlay: Messages): Messages {
  const out: Messages = { ...base };
  for (const [key, value] of Object.entries(overlay)) {
    if (key === '__proto__' || key === 'constructor') continue;
    // eslint-disable-next-line security/detect-object-injection
    const current = out[key];
    // eslint-disable-next-line security/detect-object-injection
    out[key] = isObject(current) && isObject(value) ? mergeMessages(current, value) : value;
  }
  return out;
}

const composer = i18n.global as unknown as {
  setLocaleMessage: (locale: string, messages: Messages) => void;
};
const original = composer.setLocaleMessage.bind(composer);
composer.setLocaleMessage = (locale: string, messages: Messages) => {
  const overlay = overlays.get(locale);
  original(locale, overlay ? mergeMessages(messages, overlay) : messages);
};
