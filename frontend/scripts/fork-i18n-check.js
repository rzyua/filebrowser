// fork: compares the hand-maintained overlays in src/i18n/fork/ with en.json.
// Prints new upstream strings that still need translating, stale keys, and
// placeholder mismatches. With --strict, exits 1 on stale keys or mismatches.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const dir = path.dirname(fileURLToPath(import.meta.url));
const i18nDir = path.resolve(dir, '../src/i18n');
const strict = process.argv.includes('--strict');

const flatten = (obj, prefix = '') =>
  Object.entries(obj).flatMap(([k, v]) =>
    v && typeof v === 'object' ? flatten(v, `${prefix}${k}.`) : [[`${prefix}${k}`, v]],
  );
const placeholders = (s) => (String(s).match(/\{\w+\}/g) || []).sort().join(' ');

const en = new Map(flatten(JSON.parse(fs.readFileSync(path.join(i18nDir, 'en.json'), 'utf8'))));
let problems = 0;

for (const file of fs.readdirSync(path.join(i18nDir, 'fork')).filter((f) => f.endsWith('.json'))) {
  const overlay = new Map(flatten(JSON.parse(fs.readFileSync(path.join(i18nDir, 'fork', file), 'utf8'))));
  const missing = [...en.keys()].filter((k) => !k.startsWith('languages.') && !overlay.has(k));
  const stale = [...overlay.keys()].filter((k) => !en.has(k));
  const mismatched = [...overlay.keys()].filter((k) => en.has(k) && placeholders(en.get(k)) !== placeholders(overlay.get(k)));
  problems += stale.length + mismatched.length;

  console.log(`### Translation overlay ${file}`);
  console.log(`${overlay.size} strings, ${missing.length} new upstream strings to translate, ${stale.length} stale, ${mismatched.length} placeholder mismatches.`);
  for (const [label, keys] of [['To translate', missing], ['Stale (removed upstream)', stale], ['Placeholder mismatch', mismatched]]) {
    if (!keys.length) continue;
    console.log(`\n${label}:`);
    for (const k of keys.slice(0, 50)) console.log(`- \`${k}\`: ${JSON.stringify(en.get(k) ?? overlay.get(k))}`);
    if (keys.length > 50) console.log(`- ...and ${keys.length - 50} more`);
  }
  console.log('');
}

if (strict && problems > 0) process.exit(1);
