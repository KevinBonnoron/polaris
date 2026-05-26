export type ThemeMode = 'light' | 'dark';

export interface Theme {
  key: string;
  name: string;
  mode: ThemeMode;
}

export const THEMES: Theme[] = [
  { key: 'classic-dark', name: 'Classic Dark', mode: 'dark' },
  { key: 'classic-light', name: 'Classic Light', mode: 'light' },
  { key: 'catppuccin-mocha', name: 'Catppuccin Mocha', mode: 'dark' },
  { key: 'catppuccin-latte', name: 'Catppuccin Latte', mode: 'light' },
  { key: 'dracula', name: 'Dracula', mode: 'dark' },
  { key: 'nord', name: 'Nord', mode: 'dark' },
  { key: 'tokyo-night', name: 'Tokyo Night', mode: 'dark' },
  { key: 'gruvbox', name: 'Gruvbox', mode: 'dark' },
  { key: 'rose-pine', name: 'Rosé Pine', mode: 'dark' },
  { key: 'solarized-dark', name: 'Solarized Dark', mode: 'dark' },
  { key: 'solarized-light', name: 'Solarized Light', mode: 'light' },
  { key: 'one-dark', name: 'One Dark', mode: 'dark' },
  { key: 'monokai-pro', name: 'Monokai Pro', mode: 'dark' },
  { key: 'github-dark', name: 'GitHub Dark', mode: 'dark' },
  { key: 'github-light', name: 'GitHub Light', mode: 'light' },
  { key: 'everforest', name: 'Everforest', mode: 'dark' },
  { key: 'synthwave-84', name: "Synthwave '84", mode: 'dark' },
  { key: 'night-owl', name: 'Night Owl', mode: 'dark' },
  { key: 'material-darker', name: 'Material Darker', mode: 'dark' },
  { key: 'palenight', name: 'Palenight', mode: 'dark' },
  { key: 'ayu-mirage', name: 'Ayu Mirage', mode: 'dark' },
  { key: 'ayu-light', name: 'Ayu Light', mode: 'light' },
  { key: 'atom-one-light', name: 'Atom One Light', mode: 'light' },
  { key: 'cobalt-2', name: 'Cobalt 2', mode: 'dark' },
  { key: 'shades-of-purple', name: 'Shades of Purple', mode: 'dark' },
  { key: 'andromeda', name: 'Andromeda', mode: 'dark' },
  { key: 'horizon', name: 'Horizon', mode: 'dark' },
  { key: 'kanagawa', name: 'Kanagawa', mode: 'dark' },
  { key: 'monokai-classic', name: 'Monokai Classic', mode: 'dark' },
];

export const DEFAULT_THEME_KEY = 'classic-dark';
export const LIGHT_THEME_KEYS = THEMES.filter((t) => t.mode === 'light').map((t) => t.key);
