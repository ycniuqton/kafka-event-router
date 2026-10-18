import type { ConnectionFormState } from './ConnectionForm';

/** Updates one field of the connection form. */
export type ConnectionFormSetter = <K extends keyof ConnectionFormState>(key: K, value: ConnectionFormState[K]) => void;

/** Props shared by every step and every security-mode field group of the connection panel. */
export type ConnectionFormStepProps = {
  form: ConnectionFormState;
  set: ConnectionFormSetter;
  editing: boolean;
  /** Id of the edited connection (absent while creating). */
  connectionId?: string;
  /** Server field errors by form field name (see `ConnectionFieldErrors`). */
  errors: Record<string, string>;
};
