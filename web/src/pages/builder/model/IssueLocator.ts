import { BuilderStep, type Issue } from '../../../domain';

/** Where an issue points in the builder: the step to open and a readable field label. */
export type IssueTarget = { step: BuilderStep; field: string };

/** Maps an issue's JSON Pointer (route document) to the builder step and field it belongs to (FR-31). */
export class IssueLocator {
  private static readonly prefixes: [string, BuilderStep][] = [
    ['/source', BuilderStep.Source],
    ['/branches/0/filter', BuilderStep.Filter],
    ['/branches/0/transform', BuilderStep.Transform],
    ['/branches/0/destination', BuilderStep.Destination],
    ['/errorPolicy', BuilderStep.Destination],
  ];

  private static readonly sectionSeparator = '/';
  private static readonly fieldJoiner = ' › ';

  static locate(issue: Issue): IssueTarget {
    const match = IssueLocator.prefixes.find(([prefix]) => issue.path === prefix || issue.path.startsWith(`${prefix}/`));
    const step = match ? match[1] : BuilderStep.Review;
    const rest = match ? issue.path.slice(match[0].length) : issue.path;
    return { step, field: rest.split(IssueLocator.sectionSeparator).filter(Boolean).join(IssueLocator.fieldJoiner) };
  }
}
