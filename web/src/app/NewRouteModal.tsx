import { useNavigate } from 'react-router-dom';
import { Modal, ModalWidth } from '../components/ui';
import { TemplateCatalog, type RouteTemplate } from '../domain';
import { actions, useNewRouteChooserOpen } from '../store';
import styles from './NewRouteModal.module.css';
import { AppPaths } from './paths';

/** "+ New route" chooser: four template cards that open the builder pre-filled. */
export function NewRouteModal() {
  const open = useNewRouteChooserOpen();
  const navigate = useNavigate();
  const close = () => actions.closeNewRouteChooser();
  const choose = (template: RouteTemplate) => {
    close();
    navigate(AppPaths.routeNew(template.id));
  };
  return (
    <Modal
      open={open}
      onClose={close}
      width={ModalWidth.Lg}
      title="New route"
      subtitle="Pick a starting point. Templates only pre-fill steps — you edit everything in the builder."
    >
      <div className={styles.grid}>
        {TemplateCatalog.all.map((t) => (
          <button key={t.id} type="button" className={styles.card} onClick={() => choose(t)}>
            <div className={styles.preview} aria-hidden>
              {t.blocks.map((b, i) => (
                <span key={i} className={`${styles.block} ${styles[b]}`} />
              ))}
              {t.blocks.length === 0 && <span className={styles.blank} />}
            </div>
            <div className={styles.title}>{t.title}</div>
            <div className={styles.description}>{t.description}</div>
          </button>
        ))}
      </div>
    </Modal>
  );
}
