import { ImageUp } from 'lucide-react';
import { useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Label } from '@/components/ui/label';
import { toastError } from '@/lib/toast-error';
import { ColorPicker } from './color-picker';
import { ProjectAvatar } from './project-avatar';
import { readImageAsDataUrl } from './read-image';

interface ProjectFormShape {
  name: string;
  color: string;
  logo: string | undefined;
}

// biome-ignore lint/suspicious/noExplicitAny: tanstack-react-form generics are unwieldy to thread through here
type AnyForm = any;

interface Props {
  form: AnyForm;
  colorLabel: string;
}

export function ProjectLogoColorFields({ form, colorLabel }: Props) {
  const { t } = useTranslation();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const pickLogo = async (file: File | null | undefined) => {
    if (!file) {
      return;
    }
    try {
      const dataUrl = await readImageAsDataUrl(file);
      form.setFieldValue('logo', dataUrl);
    } catch (err) {
      toastError({ title: t('projects.settings.couldNotLoadImage'), err });
    }
  };

  return (
    <>
      <form.Subscribe selector={(state: { values: ProjectFormShape }) => ({ name: state.values.name, color: state.values.color, logo: state.values.logo })}>
        {({ name, color, logo }: ProjectFormShape) => (
          <div className="flex flex-col gap-2">
            <Label className="text-xs text-muted-foreground">{t('projects.settings.logo')}</Label>
            <div className="flex items-center gap-3">
              <button
                type="button"
                onClick={() => fileInputRef.current?.click()}
                className="group relative size-16 shrink-0 cursor-pointer rounded-md outline-none ring-offset-2 ring-offset-background focus-visible:ring-2 focus-visible:ring-ring"
                aria-label={logo ? t('projects.settings.replaceLogo') : t('projects.settings.uploadLogo')}
              >
                <ProjectAvatar project={{ name, color, logo }} className="size-full rounded-md" textClassName="text-lg" />
                <span className="pointer-events-none absolute inset-0 flex items-center justify-center rounded-md bg-black/50 text-white opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100">
                  <ImageUp className="size-5" />
                </span>
              </button>
              {logo && (
                <button type="button" onClick={() => form.setFieldValue('logo', undefined)} className="text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline">
                  {t('projects.settings.remove')}
                </button>
              )}
              <input
                ref={fileInputRef}
                type="file"
                accept="image/*"
                className="hidden"
                onChange={(e) => {
                  const input = e.currentTarget;
                  pickLogo(input.files?.[0]).finally(() => {
                    input.value = '';
                  });
                }}
              />
            </div>
          </div>
        )}
      </form.Subscribe>

      <form.Subscribe selector={(state: { values: ProjectFormShape }) => state.values.logo}>
        {(logo: string | undefined) =>
          !logo && (
            <form.Field name="color">
              {(field: { state: { value: string }; handleChange: (v: string) => void }) => (
                <div className="flex flex-col gap-2">
                  <Label className="text-xs text-muted-foreground">{colorLabel}</Label>
                  <ColorPicker value={field.state.value} onChange={field.handleChange} />
                </div>
              )}
            </form.Field>
          )
        }
      </form.Subscribe>
    </>
  );
}
