import type { ButtonHTMLAttributes } from 'react';
import { ButtonSize, ButtonVariant } from './variants';
import styles from './Button.module.css';

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** Icon-only square button (pass `title` for the tooltip). */
  square?: boolean;
  block?: boolean;
};

/** The design's button family: primary (accent), secondary (outlined), ghost, danger. */
export function Button({ variant = 'secondary', size = 'md', square, block, className, type = 'button', ...rest }: ButtonProps) {
  const classes = [styles.button, styles[variant], styles[size], square && styles.square, block && styles.block, className]
    .filter(Boolean)
    .join(' ');
  return <button type={type} className={classes} {...rest} />;
}
