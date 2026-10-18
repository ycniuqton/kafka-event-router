#!/bin/sh
# Guardrail check of the orders-smoke route: one OrderPaid (must arrive masked) + one OrderCreated (must be filtered).
K="docker exec -i event-router-staging-kafka-1 /opt/kafka/bin"
ID="stresschk-$(date +%s)"
printf '%s\n' "{\"eventType\":\"OrderPaid\",\"orderId\":\"$ID-paid\",\"email\":\"alice@example.com\",\"amount\":12}" \
              "{\"eventType\":\"OrderCreated\",\"orderId\":\"$ID-created\",\"email\":\"bob@example.com\"}" \
  | $K/kafka-console-producer.sh --bootstrap-server kafka:19092 --topic order-events >/dev/null 2>&1
sleep 8
OUT=$($K/kafka-console-consumer.sh --bootstrap-server kafka:19092 --topic orders-public --from-beginning --timeout-ms 8000 2>/dev/null | grep "$ID")
PAID=$(echo "$OUT" | grep -c "$ID-paid")
CREATED=$(echo "$OUT" | grep -c "$ID-created")
CLEAR=$(echo "$OUT" | grep -c "alice@example.com")
echo "id=$ID paid_delivered=$PAID created_delivered=$CREATED email_in_clear=$CLEAR line=$(echo "$OUT" | head -1)"
[ "$PAID" = 1 ] && [ "$CREATED" = 0 ] && [ "$CLEAR" = 0 ] && echo SMOKE_OK || { echo SMOKE_FAIL; exit 1; }
