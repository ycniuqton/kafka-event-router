/** Which dialog of the routes page is open. */
export const RoutesDialog = { None: 'none', Tag: 'tag', Import: 'import' } as const;
export type RoutesDialog = (typeof RoutesDialog)[keyof typeof RoutesDialog];
