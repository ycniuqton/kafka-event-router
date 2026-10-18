/** Source of "now" for relative times; mock data uses a fixed reference instant. */
export abstract class Clock {
  abstract now(): Date;
}

/** Real wall-clock time, used with the live API. */
export class SystemClock extends Clock {
  now(): Date {
    return new Date();
  }
}
