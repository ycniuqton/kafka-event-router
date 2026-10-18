import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { services } from '../../api';
import { Button, ButtonSize, ControlSize, Drawer, DrawerWidth, EmptyState, Page, PageHeader, PageWidth, Tabs, TextInput } from '../../components/ui';
import type { LibraryLookupTable, LibraryTransform } from '../../domain';
import { useServiceQuery } from '../../store';
import { LookupTableDetail } from './components/LookupTableDetail';
import { LookupTablesList } from './components/LookupTablesList';
import { NewLookupTableDialog } from './components/NewLookupTableDialog';
import { TransformDetail } from './components/TransformDetail';
import { TransformsList } from './components/TransformsList';
import { LibraryParam, LibraryTab, LibraryText } from './model/LibraryLayout';
import listStyles from './components/LibraryList.module.css';
import styles from './LibraryPage.module.css';

/** Transform library (P3, spec §8.7): tabs for transforms and lookup tables, search, detail drawer per object. */
export default function LibraryPage() {
  const [params, setParams] = useSearchParams();
  const tab = (params.get(LibraryParam.Tab) as LibraryTab | null) ?? LibraryTab.Transforms;
  const text = params.get(LibraryParam.Search) ?? '';
  const [refresh, setRefresh] = useState(0);
  const [creating, setCreating] = useState(false);
  const transforms = useServiceQuery(() => services.library.list({ text }), [text, refresh]);
  const tables = useServiceQuery(() => services.library.lookupTables({ text }), [text, refresh]);
  const set = (key: string, value: string | null) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    setParams(next, { replace: true });
  };
  const reload = () => setRefresh(refresh + 1);
  const error = transforms.error ?? tables.error;
  return (
    <div className={styles.root}>
      <Page width={PageWidth.Library}>
        <PageHeader title={LibraryText.title} subtitle={LibraryText.subtitle} />
        {error && <EmptyState compact title={LibraryText.loadError} description={error.message} />}
        <Tabs items={LibraryText.tabs} value={tab} onChange={(id) => set(LibraryParam.Tab, id)} />
        <div className={listStyles.toolbar}>
          <TextInput controlSize={ControlSize.Md} className={listStyles.grow} placeholder={LibraryText.search} aria-label={LibraryText.search} value={text} onChange={(e) => set(LibraryParam.Search, e.target.value)} />
          {tab === LibraryTab.LookupTables && <Button size={ButtonSize.Md} onClick={() => setCreating(true)}>{LibraryText.newTable}</Button>}
        </div>
        {tab === LibraryTab.Transforms ? (
          <TransformsList transforms={transforms.data ?? []} loading={transforms.loading} searching={!!text} onOpen={(t) => set(LibraryParam.Item, t.id)} />
        ) : (
          <LookupTablesList tables={tables.data ?? []} loading={tables.loading} searching={!!text} onOpen={(t) => set(LibraryParam.Item, t.id)} />
        )}
      </Page>
      <DetailDrawer tab={tab} id={params.get(LibraryParam.Item)} onClose={() => set(LibraryParam.Item, null)} onChange={reload} />
      <NewLookupTableDialog open={creating} onClose={() => setCreating(false)} onCreated={(t) => {
          setCreating(false);
          reload();
          set(LibraryParam.Item, t.id);
        }} />
    </div>
  );
}

type DetailDrawerProps = { tab: LibraryTab; id: string | null; onClose(): void; onChange(): void };

/** Loads the open object and shows its detail in a drawer. */
function DetailDrawer({ tab, id, onClose, onChange }: DetailDrawerProps) {
  const isTable = tab === LibraryTab.LookupTables;
  const [local, setLocal] = useState<LibraryTransform | LibraryLookupTable | null>(null);
  const loaded = useServiceQuery<LibraryTransform | LibraryLookupTable | null>(
    () => (!id ? Promise.resolve(null) : isTable ? services.library.getLookupTable(id) : services.library.getTransform(id)),
    [id, isTable],
  );
  const item = local?.id === id ? local : (loaded.data ?? null);
  const changed = (next: LibraryTransform | LibraryLookupTable) => {
    setLocal(next);
    onChange();
  };
  const archived = () => {
    onChange();
    onClose();
  };
  return (
    <Drawer open={!!id && !!item} onClose={onClose} title={item?.name ?? ''} width={DrawerWidth.Wide}>
      {item && isTable && <LookupTableDetail table={item as LibraryLookupTable} onChange={changed} onArchived={archived} />}
      {item && !isTable && <TransformDetail transform={item} onChange={changed} onArchived={archived} />}
    </Drawer>
  );
}
