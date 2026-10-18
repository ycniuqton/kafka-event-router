import { test } from '@playwright/test';
import { AccApi } from '../lib/Api';
import { DemoApprover, DemoGateFlow, DemoSeeder } from './DemoSeeder';

/** Seeds (idempotently) the two-cluster demo on staging: connections, topics, routes, test cases, alert, 4-eyes. */
test('seed the staging two-cluster demo', async ({ context, browser }) => {
  const seeder = new DemoSeeder(await AccApi.signIn(context));
  await test.step('connections kafka-a / kafka-b / kafka-b-prod', () => seeder.connections());
  await test.step('demo topics (3 partitions)', () => seeder.topics());
  await test.step('demo routes running', () => seeder.routes());
  await test.step('saved test cases green', () => seeder.testCases());
  await test.step('alert rule', () => seeder.alertRule());
  const approver = await test.step('approver identity', () => new DemoApprover(seeder.api).signIn(browser));
  await test.step('4-eyes gate route', () => new DemoGateFlow(seeder, approver).run());
  console.log('route ids', JSON.stringify(seeder.routeIds), 'connections', JSON.stringify(seeder.conn));
});
