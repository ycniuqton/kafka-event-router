import { Button } from '../../../../components/ui';
import { DocumentFormat, type Route } from '../../../../domain';
import { actions } from '../../../../store';
import { RouteFileSaver } from '../../../routes/model/RouteFileSaver';

/** Header "Export YAML" / "JSON": downloads the route (live version, else draft) with connection names. */
export function ExportAction({ route }: { route: Route }) {
  const save = (format: DocumentFormat) =>
    void actions.routes.exportRoutes([route.id], format).then(RouteFileSaver.save, (e: Error) => actions.toast.show(e.message));
  return (
    <>
      <Button onClick={() => save(DocumentFormat.Yaml)}>Export YAML</Button>
      <Button variant="ghost" onClick={() => save(DocumentFormat.Json)}>JSON</Button>
    </>
  );
}
