import { useParams, useSearchParams } from 'react-router-dom';
import { services } from '../../api';
import { QueryParam } from '../../app/paths';
import type { RouteTemplateId } from '../../domain';
import { useServiceQuery } from '../../store';
import { BuilderWorkspace } from './components/BuilderWorkspace';
import styles from './BuilderPage.module.css';

/** Route builder for `/routes/new?template=` and `/routes/:id/edit?step=&sample=` (spec §7, §9). */
export default function BuilderPage() {
  const { id } = useParams();
  const [params] = useSearchParams();
  const routeId = id ?? null;
  const template = params.get(QueryParam.Template) as RouteTemplateId | null;
  const draft = useServiceQuery(() => services.routes.openDraft(routeId, template), [routeId, template]);
  const fields = useServiceQuery(() => services.samples.fields(), []);
  const ready = draft.data && fields.data && draft.data.routeId === routeId;
  if (!ready || !draft.data || !fields.data) {
    return <div className={styles.loading}>{draft.error ? draft.error.message : 'Loading route…'}</div>;
  }
  return <BuilderWorkspace key={`${routeId}:${template}`} initialDraft={draft.data} fields={fields.data} />;
}
