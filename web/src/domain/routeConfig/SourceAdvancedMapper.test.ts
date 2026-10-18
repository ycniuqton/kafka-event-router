import { describe, expect, it } from 'vitest';
import type { RouteConfig } from '../../api/generated';
import { SourceFormatType, StartPosition } from '../builder';
import { RouteStatus } from '../route';
import { RouteDraftMapper } from './RouteDraftMapper';

const meta = { routeId: 'r1', name: 'audit', status: RouteStatus.Draft };
const doc: RouteConfig = {
  apiVersion: 'router/v1',
  name: 'audit',
  source: {
    connectionId: 'c1',
    topicPattern: '^audit\\..*$',
    format: { type: 'protobuf', subject: 'audit-value', messageType: 'acme.Audit' },
    startFrom: { mode: 'offsets', offsets: { '0': 42, '1': 7 } },
  },
  branches: [{ id: 'main', destination: { connectionId: 'c2', topic: 'audit-all', format: { type: 'avro', subject: 'audit-all-value' } } }],
};

describe('RouteDraftMapper · source advanced (routes#p2-fe-source-advanced)', () => {
  it('reads topic pattern, offsets and the Protobuf format', () => {
    const { draft } = RouteDraftMapper.fromDocument(doc, meta);
    expect(draft.source.topicPattern).toBe('^audit\\..*$');
    expect(draft.source.start).toBe(StartPosition.Offsets);
    expect(draft.source.offsets).toEqual({ '0': 42, '1': 7 });
    expect(draft.source.valueFormat).toEqual({ type: SourceFormatType.Protobuf, subject: 'audit-value', messageType: 'acme.Audit' });
    expect(draft.destination.schemaSubject).toBe('audit-all-value');
  });

  it('round-trips them, and edits replace them', () => {
    const { draft, snapshot } = RouteDraftMapper.fromDocument(doc, meta);
    const same = RouteDraftMapper.toDocument(draft, snapshot);
    expect(same.source).toEqual(doc.source);
    const edited = RouteDraftMapper.toDocument(
      { ...draft, source: { ...draft.source, topicPattern: '', topics: ['audit.eu'], offsets: { '3': 1 }, valueFormat: { type: SourceFormatType.Avro, subject: '' } } },
      snapshot,
    );
    expect(edited.source.topicPattern).toBeUndefined();
    expect(edited.source.topics).toEqual(['audit.eu']);
    expect(edited.source.startFrom).toEqual({ mode: 'offsets', offsets: { '3': 1 } });
    expect(edited.source.format).toEqual({ type: 'avro' });
  });
});
