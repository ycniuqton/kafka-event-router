import { useToast } from '../../store/hooks';
import styles from './ToastHost.module.css';

/** Renders the current toast from the store. Mounted once by the app shell; use `actions.toast.show()`. */
export function ToastHost() {
  const toast = useToast();
  if (!toast) return null;
  return (
    <div key={toast.id} className={styles.toast} role="status" aria-live="polite">
      {toast.message}
    </div>
  );
}
