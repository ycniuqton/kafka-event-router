// Package dlq is the read/act side of the dead-letter queue: browse DLQ topics without a consumer
// group, keep resolved state in a compacted topic, retry through the active route version, discard
// and export. HTTP lives in dlq/httpapi; the shared record contract in dlq/record.
package dlq
