interface WTermLike {
  element?: HTMLElement;
  _scrollToBottom?: () => void;
}

// @wterm rounds scrollTop down to a row boundary, which both leaves a few pixels
// at the bottom and breaks its own "stick to bottom" detection. Scroll exactly.
export function patchScrollToBottom(instance: unknown): HTMLElement | null {
  const inst = instance as WTermLike | null | undefined;
  const el = inst?.element ?? null;
  if (inst && el) {
    inst._scrollToBottom = () => {
      el.scrollTop = el.scrollHeight;
    };
  }
  return el;
}
