import type { SettingsFormValues } from '../../model/SettingsForm';

/** Props shared by the Settings form sections. */
export type SettingsSectionProps = {
  values: SettingsFormValues;
  errors: Record<string, string>;
  onChange: (patch: Partial<SettingsFormValues>) => void;
  readOnly: boolean;
};
