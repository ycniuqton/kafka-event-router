import { ExternalStore } from '../ExternalStore';
import { PreviewStateFactory, type PreviewState } from './PreviewState';

/** Observable state of one sample panel (one per builder / studio instance). */
export class PreviewStore extends ExternalStore<PreviewState> {
  constructor() {
    super(PreviewStateFactory.initial());
  }
}
