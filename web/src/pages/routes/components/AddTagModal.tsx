import { useState } from 'react';
import { Button, Field, Modal, TextInput } from '../../../components/ui';

type AddTagModalProps = { open: boolean; count: number; onClose: () => void; onConfirm: (tag: string) => void };

/** Asks for a tag name to add to the selected routes (tags are stubbed in the demo). */
export function AddTagModal({ open, count, onClose, onConfirm }: AddTagModalProps) {
  const [tag, setTag] = useState('');
  const submit = () => {
    if (!tag.trim()) return;
    onConfirm(tag.trim());
    setTag('');
  };
  const footer = (
    <>
      <Button size="lg" onClick={onClose}>Cancel</Button>
      <Button size="lg" variant="primary" onClick={submit} disabled={!tag.trim()}>Add tag</Button>
    </>
  );
  return (
    <Modal open={open} onClose={onClose} title="Add tag" subtitle={`${count} routes selected`} footer={footer}>
      <Field label="Tag" hint="Tags group routes for filtering and ownership.">
        <TextInput autoFocus block mono value={tag} onChange={(e) => setTag(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && submit()} placeholder="team:payments" />
      </Field>
    </Modal>
  );
}
