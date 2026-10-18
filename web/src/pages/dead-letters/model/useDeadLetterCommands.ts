import { useNavigate } from 'react-router-dom';
import { AppPaths } from '../../../app/paths';
import { BuilderStep, DeadLetterFileFormat, type DeadLetter, type DeadLetterSelection } from '../../../domain';
import { actions } from '../../../store';
import { DeadLetterExporter } from './DeadLetterExporter';

/** Studio and export commands shared by the detail panel and the toolbar. */
export function useDeadLetterCommands() {
  const navigate = useNavigate();
  return {
    openInStudio: async (d: DeadLetter) => {
      const link = await actions.deadLetters.openInStudio(d);
      if (link) navigate(link.builderUrl ?? AppPaths.routeEdit(link.routeId, BuilderStep.Transform, link.sampleId));
    },
    exportJson: async (routeId: string, selection: DeadLetterSelection, format: DeadLetterFileFormat = DeadLetterFileFormat.Json) => {
      const file = await actions.deadLetters.exportJson(routeId, selection, format);
      if (!file) return;
      DeadLetterExporter.download(file);
      actions.toast.show(DeadLetterExporter.toast(format));
    },
  };
}
