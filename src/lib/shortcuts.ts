import type { polaris } from '@/wailsjs/go/models';

export type ShortcutId = 'openPalette' | 'switchProject' | 'addProject' | 'newAgent' | 'toggleSidebar' | 'toggleTerminal' | 'closeModal';

export interface ShortcutDef {
  id: ShortcutId;
  key: string;
  meta: boolean;
}

export const SHORTCUT_IDS: ShortcutId[] = ['openPalette', 'switchProject', 'addProject', 'newAgent', 'toggleSidebar', 'toggleTerminal', 'closeModal'];

export const DEFAULT_SHORTCUTS: Record<ShortcutId, ShortcutDef> = {
  openPalette: { id: 'openPalette', key: 'k', meta: true },
  switchProject: { id: 'switchProject', key: 'p', meta: true },
  addProject: { id: 'addProject', key: 'o', meta: true },
  newAgent: { id: 'newAgent', key: 'n', meta: true },
  toggleSidebar: { id: 'toggleSidebar', key: 'b', meta: true },
  toggleTerminal: { id: 'toggleTerminal', key: 'j', meta: true },
  closeModal: { id: 'closeModal', key: 'Escape', meta: false },
};

export function fromBackend(s: polaris.ShortcutsSettings): Record<ShortcutId, ShortcutDef> {
  return {
    openPalette: { id: 'openPalette', key: s.openPalette?.key ?? 'k', meta: s.openPalette?.meta ?? true },
    switchProject: { id: 'switchProject', key: s.switchProject?.key ?? 'p', meta: s.switchProject?.meta ?? true },
    addProject: { id: 'addProject', key: s.addProject?.key ?? 'o', meta: s.addProject?.meta ?? true },
    newAgent: { id: 'newAgent', key: s.newAgent?.key ?? 'n', meta: s.newAgent?.meta ?? true },
    toggleSidebar: { id: 'toggleSidebar', key: s.toggleSidebar?.key ?? 'b', meta: s.toggleSidebar?.meta ?? true },
    toggleTerminal: { id: 'toggleTerminal', key: s.toggleTerminal?.key ?? 'j', meta: s.toggleTerminal?.meta ?? true },
    closeModal: { id: 'closeModal', key: s.closeModal?.key ?? 'Escape', meta: s.closeModal?.meta ?? false },
  };
}

export function toBackend(shortcuts: Record<ShortcutId, ShortcutDef>): polaris.ShortcutsSettings {
  return {
    openPalette: { key: shortcuts.openPalette.key, meta: shortcuts.openPalette.meta },
    switchProject: { key: shortcuts.switchProject.key, meta: shortcuts.switchProject.meta },
    addProject: { key: shortcuts.addProject.key, meta: shortcuts.addProject.meta },
    newAgent: { key: shortcuts.newAgent.key, meta: shortcuts.newAgent.meta },
    toggleSidebar: { key: shortcuts.toggleSidebar.key, meta: shortcuts.toggleSidebar.meta },
    toggleTerminal: { key: shortcuts.toggleTerminal.key, meta: shortcuts.toggleTerminal.meta },
    closeModal: { key: shortcuts.closeModal.key, meta: shortcuts.closeModal.meta },
  } as polaris.ShortcutsSettings;
}

export function matchesShortcut(e: KeyboardEvent, def: ShortcutDef, isMac: boolean): boolean {
  const modifierHeld = isMac ? e.metaKey : e.ctrlKey;
  const otherModifierHeld = isMac ? e.ctrlKey : e.metaKey;
  if (def.meta && (!modifierHeld || otherModifierHeld)) {
    return false;
  }
  if (!def.meta && (e.metaKey || e.ctrlKey)) {
    return false;
  }
  return e.key.toLowerCase() === def.key.toLowerCase();
}

export function shortcutDisplayKeys(def: ShortcutDef, modifierSymbol: string): string[] {
  if (!def.meta) {
    return [def.key === 'Escape' ? 'Esc' : def.key];
  }
  return [modifierSymbol, def.key.toUpperCase()];
}

export function detectMac(): boolean {
  return /Mac|iPhone|iPad|iPod/.test(navigator.platform);
}
