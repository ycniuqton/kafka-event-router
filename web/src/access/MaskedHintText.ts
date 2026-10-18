/** Texts of the masked-data hint (decision: Viewers always masked; everyone masked on sensitive connections). */
export const MaskedHintText = {
  Title: 'Masked for your role',
  Viewer: 'Viewers always see masked values; types are kept so filters and steps still work.',
  Sensitive: 'This connection is marked sensitive, so values are masked for everyone.',
} as const;
