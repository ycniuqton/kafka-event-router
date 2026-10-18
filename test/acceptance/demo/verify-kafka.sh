#!/usr/bin/env bash
# End-to-end check of the staging demo routes, run ON the staging host (see DEMO-STAGING.md):
#   ssh root@<host> 'bash -s' < test/acceptance/demo/verify-kafka.sh
# Produces uniquely tagged events with the Kafka CLI (throw-away apache/kafka container on the backend network,
# so the brokers' memory limits are untouched) and checks what each route wrote. Read-only for everything else.
set -euo pipefail

NET=event-router-staging-backend
IMAGE=apache/kafka:3.9.1
A=kafka:19092
B=kafka-b:19092
RUN="v$(date +%s)"
WAIT_SECONDS=20
READ_LIMIT_SECONDS=120
FAILS=0

# Runs a Kafka CLI tool in a small throw-away container (stdin is passed through: redirect it when unused,
# because this script itself may arrive on stdin via `bash -s`).
kcli() {
  docker run --rm -i --network "$NET" --memory 256m -e KAFKA_HEAP_OPTS="-Xmx128m" "$IMAGE" "$@"
}

# produce <bootstrap> <topic>: stdin lines "key|json".
produce() {
  kcli /opt/kafka/bin/kafka-console-producer.sh --bootstrap-server "$1" --topic "$2" \
    --property parse.key=true --property key.separator='|' > /dev/null
}

# end_count <bootstrap> <topic>: sum of the partitions' end offsets (bounds a read while traffic keeps flowing).
end_count() {
  kcli /opt/kafka/bin/kafka-get-offsets.sh --bootstrap-server "$1" --topic "$2" --time -1 < /dev/null 2> /dev/null |
    awk -F: '{ s += $3 } END { print s + 0 }'
}

# consume <bootstrap> <topic>: prints "key|value" lines of this run (read_committed, from the beginning up to
# the current end offsets; bounded by READ_LIMIT_SECONDS).
consume() {
  local count
  count="$(end_count "$1" "$2")"
  [ "$count" -gt 0 ] || return 0
  timeout "$READ_LIMIT_SECONDS" docker run --rm --network "$NET" --memory 256m -e KAFKA_HEAP_OPTS="-Xmx128m" "$IMAGE" \
    /opt/kafka/bin/kafka-console-consumer.sh --bootstrap-server "$1" --topic "$2" --from-beginning \
    --max-messages "$count" --timeout-ms 15000 --isolation-level read_committed \
    --property print.key=true --property key.separator='|' 2> /dev/null < /dev/null | grep -F -- "$RUN" || true
}

# check <label> <haystack> <grep -E pattern> [absent]: one PASS/FAIL line.
check() {
  local label="$1" data="$2" pattern="$3" mode="${4:-present}" found=no
  grep -Eq -- "$pattern" <<< "$data" && found=yes
  if { [ "$mode" = present ] && [ "$found" = yes ]; } || { [ "$mode" = absent ] && [ "$found" = no ]; }; then
    echo "PASS  $label"
  else
    echo "FAIL  $label"; FAILS=$((FAILS + 1))
  fi
}

echo "run tag: $RUN"
produce "$A" demo-orders <<EOF
ord-$RUN-1|{"seq":1,"orderId":"ord-$RUN-1","eventType":"OrderPaid","customer":{"id":"cus-900","email":"verify.user@example.com"},"amount":42.5,"currency":"EUR","items":1}
ord-$RUN-2|{"seq":2,"orderId":"ord-$RUN-2","eventType":"OrderCreated","customer":{"id":"cus-900","email":"verify.user@example.com"},"amount":15,"currency":"EUR","items":1}
ord-$RUN-3|{"seq":3,"orderId":"ord-$RUN-3","eventType":"OrderCancelled","customer":{"id":"cus-900","email":"verify.user@example.com"},"amount":15,"currency":"EUR","items":1}
ord-$RUN-4|{"seq":4,"orderId":"ord-$RUN-4","eventType":"OrderShipped","customer":{"id":"cus-900","email":"verify.user@example.com"},"amount":500,"currency":"EUR","items":3}
EOF
produce "$B" demo-payments <<EOF
pay-$RUN-1|{"seq":1,"paymentId":"pay-$RUN-1","orderId":"ord-$RUN-1","amount_cents":31000,"currency":"EUR","method":"card","status":"captured","internal_debug":{"gateway":"verify"}}
EOF
produce "$B" demo-raw <<EOF
rd-$RUN-1|{"seq":1,"id":"rd-$RUN-1","sensor":"verify","reading":1.5,"bad":false}
rd-$RUN-2|{"seq":2,"id":"rd-$RUN-2","sensor":"verify","reading":9.9,"bad":true}
EOF
produce "$B" demo-b-in <<EOF
loc-$RUN-1|{"seq":1,"id":"loc-$RUN-1","kind":"verify"}
EOF
echo "produced; waiting ${WAIT_SECONDS}s for the routes"
sleep "$WAIT_SECONDS"

paid="$(consume "$B" demo-orders-paid)"
check "a demo-a-to-b-paid: OrderPaid on B, key=orderId" "$paid" "^ord-$RUN-1\|"
check "a demo-a-to-b-paid: email masked" "$paid" '"email":"v\*+@example.com"'
check "a demo-a-to-b-paid: OrderCreated filtered out" "$paid" "ord-$RUN-2" absent
enriched="$(consume "$A" demo-payments-enriched)"
check "b demo-b-to-a-enrich: amount computed (310)" "$enriched" '"amount":310'
check "b demo-b-to-a-enrich: status renamed" "$enriched" '"paymentStatus":"captured"'
check "b demo-b-to-a-enrich: JS step added riskLevel" "$enriched" '"riskLevel":"review"'
check "b demo-b-to-a-enrich: internal_debug dropped" "$enriched" 'internal_debug' absent
paidB="$(consume "$B" demo-paid-b)"
cancelledA="$(consume "$A" demo-cancelled-a)"
check "c demo-a-branches: OrderPaid -> B demo-paid-b" "$paidB" "^ord-$RUN-1\|"
check "c demo-a-branches: OrderCancelled -> A demo-cancelled-a" "$cancelledA" "^ord-$RUN-3\|"
check "c demo-a-branches: OrderCreated/Shipped unmatched" "$paidB$cancelledA" "ord-$RUN-(2|4)" absent
clean="$(consume "$A" demo-clean)"
dlq="$(consume "$B" router.dlq.demo-b-dlq)"
check "d demo-b-dlq: good reading -> A demo-clean (validated)" "$clean" "rd-$RUN-1.*\"validated\":true"
check "d demo-b-dlq: bad reading not delivered" "$clean" "rd-$RUN-2" absent
check "d demo-b-dlq: bad reading in router.dlq.demo-b-dlq (B)" "$dlq" "rd-$RUN-2"
local_out="$(consume "$B" demo-b-out)"
check "e demo-b-local: exactly_once B -> B, tagged" "$local_out" "loc-$RUN-1.*\"via\":\"demo-b-local\""
gate="$(consume "$B" demo-orders-highvalue)"
check "gate demo-approval-gate (v3, amount >= 100) -> B prod" "$gate" "^ord-$RUN-4\|"
check "gate demo-approval-gate: small orders filtered" "$gate" "ord-$RUN-(1|2|3)" absent

echo "failures: $FAILS"
[ "$FAILS" -eq 0 ]
