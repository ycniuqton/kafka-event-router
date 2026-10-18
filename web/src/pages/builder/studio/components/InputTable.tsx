import { Fragment } from 'react';
import { DataTable, TableCell, TableHead, TableRow } from '../../../../components/ui';
import { useStudio } from '../hooks/StudioContext';
import { JsonLineKind } from '../model/JsonKind';
import { FieldActionBar } from './FieldActionBar';
import { JsonValueText } from './JsonValueText';
import { TypeIcon } from './TypeIcon';
import styles from './InputTable.module.css';

/** Grid columns of the input table: path, type icon, value. */
class InputTableLayout {
  static readonly columns = 'minmax(0,1.1fr) 28px minmax(0,1fr)';
}

/** Table view of the current sample: one row per field path with type and value. */
export function InputTable() {
  const { model, ui, commands } = useStudio();
  return (
    <div className={styles.wrap}>
      <DataTable columns={InputTableLayout.columns}>
        <TableHead>
          <TableCell>Field</TableCell>
          <TableCell>Type</TableCell>
          <TableCell>Value</TableCell>
        </TableHead>
        {model.inputLines.map((line) => (
          <Fragment key={line.path}>
            <TableRow selected={ui.selectedLine === line.path} onClick={() => commands.selectLine(ui.selectedLine === line.path ? null : line.path)}>
              <TableCell className={styles.path} title={line.path}>
                {line.path}
              </TableCell>
              <TableCell>
                <TypeIcon kind={JsonLineKind.of(line)} />
              </TableCell>
              <TableCell className={styles.value}>
                <JsonValueText line={line} />
              </TableCell>
            </TableRow>
            {ui.selectedLine === line.path && (
              <div className={styles.actions}>
                <FieldActionBar line={line} />
              </div>
            )}
          </Fragment>
        ))}
      </DataTable>
    </div>
  );
}
