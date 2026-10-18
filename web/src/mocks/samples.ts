import type { OrderEventValue, SampleEvent } from '../domain';

/** Generates the 24 deterministic `order-events` samples of the design (`SAMPLES`). */
class SampleEventFactory {
  static readonly count = 24;
  static readonly topic = 'order-events';
  static readonly names = ['linh.tran', 'minh.nguyen', 'an.pham', 'hoa.le', 'duc.vo', 'mai.do', 'khoa.bui', 'thu.ho'];
  static readonly types = ['OrderPaid', 'OrderCreated', 'OrderShipped', 'CartUpdated', 'OrderPaid', 'OrderRefunded', 'OrderCreated', 'OrderPaid'];
  static readonly amounts = [249.5, 38, 1290, 12.99, 174.2, 560, 145, 88, 310.4, 19.9, 420, 64, 132.75, 8.5, 96, 705];
  static readonly countries = ['VN', 'VN', 'SG', 'TH', 'VN', 'US'];
  static readonly currencies = ['USD', 'USD', 'SGD', 'USD', 'EUR'];
  static readonly skus = ['SKU-1042', 'SKU-2210', 'SKU-0087', 'SKU-3391', 'SKU-7720'];
  static readonly partitions = 6;
  static readonly baseOffset = 884120;
  static readonly offsetStride = 7;
  static readonly baseOrder = 8812;
  static readonly orderStride = 13;
  static readonly baseCustomer = 1040;
  static readonly traceMultiplier = 7919;
  static readonly traceModulo = 99999;
  static readonly hexRadix = 16;
  static readonly firstMinute = 14;
  static readonly mobileEvery = 3;
  static readonly brokenSampleIndex = 6;

  static createAll(): SampleEvent[] {
    const all = Array.from({ length: SampleEventFactory.count }, (_, i) => SampleEventFactory.create(i));
    const broken = all[SampleEventFactory.brokenSampleIndex];
    broken.value.eventType = 'OrderPaid';
    delete broken.value.items;
    return all;
  }

  private static create(i: number): SampleEvent {
    const orderId = `ord_${SampleEventFactory.baseOrder + i * SampleEventFactory.orderStride}`;
    return {
      id: i,
      topic: SampleEventFactory.topic,
      partition: i % SampleEventFactory.partitions,
      offset: SampleEventFactory.baseOffset + i * SampleEventFactory.offsetStride,
      timestamp: `2026-10-13T09:${String(SampleEventFactory.firstMinute + i).padStart(2, '0')}:0${i % 10}Z`,
      key: orderId,
      headers: {
        source: i % SampleEventFactory.mobileEvery === 2 ? 'mobile-api' : 'checkout-svc',
        traceId: `tr-${((i * SampleEventFactory.traceMultiplier) % SampleEventFactory.traceModulo).toString(SampleEventFactory.hexRadix)}`,
      },
      value: SampleEventFactory.value(i, orderId),
    };
  }

  private static value(i: number, orderId: string): OrderEventValue {
    const f = SampleEventFactory;
    const amount = f.amounts[i % f.amounts.length];
    const qty = 1 + (i % 3);
    return {
      eventType: f.types[i % f.types.length],
      orderId,
      cust_id: `c_${f.baseCustomer + i * 3}`,
      email: `${f.names[i % f.names.length]}@mail.com`,
      amount,
      currency: f.currencies[i % f.currencies.length],
      country: f.countries[i % f.countries.length],
      items: [{ sku: f.skus[i % f.skus.length], qty, price: +(amount / qty).toFixed(2) }],
    };
  }
}

export const SAMPLES: SampleEvent[] = SampleEventFactory.createAll();

/** Fields offered by the filter builder, and which of them are numeric (design `FIELDS`, `NUMF`). */
export class SampleSchema {
  static readonly fields = [
    'value.eventType',
    'value.orderId',
    'value.cust_id',
    'value.email',
    'value.amount',
    'value.currency',
    'value.country',
    'headers.source',
    'key',
    'partition',
  ];
  static readonly numericFields = new Set(['value.amount', 'partition']);

  static isNumeric(field: string): boolean {
    return SampleSchema.numericFields.has(field);
  }
}
