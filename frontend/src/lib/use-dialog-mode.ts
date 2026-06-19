import { useState } from 'react';

export interface DialogModeProps {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}

export function useDialogMode({ open: openProp, onOpenChange: onOpenChangeProp }: DialogModeProps) {
  const [internalOpen, setInternalOpen] = useState(false);
  const isControlled = openProp !== undefined;
  return {
    open: isControlled ? openProp : internalOpen,
    setOpen: isControlled ? (onOpenChangeProp ?? (() => {})) : setInternalOpen,
  };
}
