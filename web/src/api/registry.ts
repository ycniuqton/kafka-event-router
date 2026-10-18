import { ApiConfig, DataSource } from './config';
import type { Services } from './contracts';
import { HttpServicesFactory } from './http/core/HttpServicesFactory';
import { MockServicesFactory } from './mock';

type ServicesFactory = () => Services;

/**
 * The service Manager: maps a configured data source to a factory and builds the active set once.
 * Add the REST implementation by registering `DataSource.Http` — no screen code changes.
 */
export class ServiceRegistry {
  private static readonly factories = new Map<string, ServicesFactory>([
    [DataSource.Mock, () => MockServicesFactory.create()],
    [DataSource.Http, () => HttpServicesFactory.create()],
  ]);

  static register(source: DataSource, factory: ServicesFactory): void {
    ServiceRegistry.factories.set(source, factory);
  }

  static resolve(source: string): Services {
    const factory = ServiceRegistry.factories.get(source);
    if (!factory) throw new Error(`No services registered for data source "${source}"`);
    return factory();
  }
}

/** The active services. Screens and the store import only this. */
export const services: Services = ServiceRegistry.resolve(ApiConfig.dataSource());
