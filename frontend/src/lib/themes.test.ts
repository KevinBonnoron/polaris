import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'bun:test';
import { THEMES } from './themes';

// THEMES (the picker list) and the .theme-* classes in styles.css are kept in
// sync by hand: a theme listed here without a matching CSS class renders with no
// colours, and a CSS class with no entry is dead. Guard the parity so adding a
// theme on one side fails loudly.
describe('theme parity', () => {
  const css = readFileSync(fileURLToPath(new URL('../styles.css', import.meta.url)), 'utf8');
  const cssKeys = new Set([...css.matchAll(/\.theme-([a-z0-9-]+)\s*\{/g)].map((m) => m[1]));
  const themeKeys = new Set(THEMES.map((t) => t.key));

  it('every THEMES entry has a matching .theme-* CSS class', () => {
    const missing = [...themeKeys].filter((k) => !cssKeys.has(k));
    expect(missing).toEqual([]);
  });

  it('every .theme-* CSS class has a matching THEMES entry', () => {
    const orphaned = [...cssKeys].filter((k) => !themeKeys.has(k));
    expect(orphaned).toEqual([]);
  });
});
