import type { InputHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from 'react';
import { ControlSize } from './variants';
import styles from './Field.module.css';

type ControlLook = { controlSize?: ControlSize; mono?: boolean; block?: boolean; sunken?: boolean };

/** Shared class list for inputs, selects and textareas. */
class ControlClasses {
  static of({ controlSize = 'md', mono, block, sunken }: ControlLook, ...extra: (string | undefined)[]): string {
    return [styles.control, controlSize !== ControlSize.Md && styles[controlSize], mono && styles.mono, block && styles.block, sunken && styles.sunken, ...extra]
      .filter(Boolean)
      .join(' ');
  }
}

export type TextInputProps = InputHTMLAttributes<HTMLInputElement> & ControlLook;

export function TextInput({ controlSize, mono, block, sunken, className, ...rest }: TextInputProps) {
  return <input className={ControlClasses.of({ controlSize, mono, block, sunken }, className)} {...rest} />;
}

export type SelectProps = SelectHTMLAttributes<HTMLSelectElement> & ControlLook;

export function Select({ controlSize, mono, block, sunken, className, ...rest }: SelectProps) {
  return <select className={ControlClasses.of({ controlSize, mono, block, sunken }, className)} {...rest} />;
}

export type TextAreaProps = TextareaHTMLAttributes<HTMLTextAreaElement> & ControlLook;

export function TextArea({ controlSize, mono, block = true, sunken = true, className, ...rest }: TextAreaProps) {
  return <textarea className={ControlClasses.of({ controlSize, mono, block, sunken }, styles.textarea, className)} {...rest} />;
}

export type FieldProps = { label: ReactNode; hint?: ReactNode; error?: ReactNode; children: ReactNode };

/** Label + control + hint/error stack for forms (create connection, deploy note). */
export function Field({ label, hint, error, children }: FieldProps) {
  return (
    <label className={styles.field}>
      <span className={styles.label}>{label}</span>
      {children}
      {error ? <span className={styles.error}>{error}</span> : hint && <span className={styles.hint}>{hint}</span>}
    </label>
  );
}
