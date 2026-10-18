import { useState } from 'react';
import { Button, Field, Modal, TextInput } from '../../../components/ui';
import { actions } from '../../../store';
import { TopicBrowserModel, type TopicForm } from './TopicBrowserModel';
import styles from './TopicBrowser.module.css';

export type CreateTopicDialogProps = { connectionId: string; open: boolean; onClose: () => void; onCreated: (name: string) => void };

/** Create topic: name, partitions (default 6), retention (default 7 d) and optional replication factor (CN-51). */
export function CreateTopicDialog({ connectionId, open, onClose, onCreated }: CreateTopicDialogProps) {
  const [form, setForm] = useState<TopicForm>(() => TopicBrowserModel.emptyForm());
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const set = (key: keyof TopicForm, value: string) => setForm((f) => ({ ...f, [key]: value }));
  const submit = async () => {
    const local = TopicBrowserModel.validate(form);
    setErrors(local);
    if (Object.keys(local).length) return;
    setBusy(true);
    try {
      await actions.connections.createTopic(connectionId, form.name, TopicBrowserModel.settings(form));
      setForm(TopicBrowserModel.emptyForm());
      onCreated(form.name);
    } catch (error) {
      setErrors(TopicBrowserModel.serverErrors(error));
    } finally {
      setBusy(false);
    }
  };
  const footer = (
    <>
      <Button size="lg" onClick={onClose}>
        Cancel
      </Button>
      <Button size="lg" variant="primary" disabled={busy} onClick={() => void submit()}>
        Create topic
      </Button>
    </>
  );
  return (
    <Modal open={open} onClose={onClose} title="Create topic" footer={footer}>
      <div className={styles.form}>
        <Field label="Name" error={errors.name}>
          <TextInput controlSize="lg" mono sunken value={form.name} placeholder="orders.paid" onChange={(e) => set('name', e.target.value.trim())} />
        </Field>
        <Field label="Partitions" error={errors.partitions}>
          <TextInput controlSize="lg" mono sunken type="number" min={1} value={form.partitions} onChange={(e) => set('partitions', e.target.value)} />
        </Field>
        <Field label="Retention (days)" error={errors.retentionMs}>
          <TextInput controlSize="lg" mono sunken type="number" min={1} value={form.retentionDays} onChange={(e) => set('retentionDays', e.target.value)} />
        </Field>
        <Field label="Replication factor" hint="Leave empty for the broker default." error={errors.replicationFactor}>
          <TextInput controlSize="lg" mono sunken type="number" min={1} value={form.replicationFactor} onChange={(e) => set('replicationFactor', e.target.value)} />
        </Field>
      </div>
    </Modal>
  );
}
