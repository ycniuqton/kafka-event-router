import { PatchPointer, PreviewFailureKind, type PreviewFailure } from '../../domain';

/** Names shown in panel messages: the connection's name (never its id) and the requested topics. */
export type PanelSourceNames = { connectionName: string; topics: string[] };

/** User-facing texts of load and run failures (design §7 message patterns). */
export class PanelMessages {
  static readonly noSource = 'Pick a source connection and topic to load samples, or upload a JSON file.';
  static readonly loading = 'Loading samples…';
  static readonly running = 'Running preview…';
  static readonly reformatting = 'Decoding samples…';

  static loadFailure(failure: PreviewFailure, names: PanelSourceNames): string {
    const conn = names.connectionName;
    switch (failure.kind) {
      case PreviewFailureKind.Forbidden:
        return `You can't read samples from ${conn}. Ask an admin for access.`;
      case PreviewFailureKind.TopicNotFound:
        return `Topic ${PanelMessages.topicAt(failure.path, names.topics)} doesn't exist on ${conn}.`;
      case PreviewFailureKind.SourceUnavailable:
        return `Can't reach ${conn}. ${failure.message} Test the connection.`;
      default:
        return failure.message;
    }
  }

  static runFailure(failure: PreviewFailure): string {
    if (failure.kind === PreviewFailureKind.InvalidConfig) {
      return failure.path ? `${failure.message} (${failure.path})` : failure.message;
    }
    if (failure.kind === PreviewFailureKind.Busy) return 'Preview is busy, retry in a moment.';
    return failure.message;
  }

  private static topicAt(path: string | undefined, topics: string[]): string {
    const index = Number(PatchPointer.parse(path ?? '').pop());
    return topics[Number.isInteger(index) ? index : 0] ?? topics.join(', ');
  }
}
