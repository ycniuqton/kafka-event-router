package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/pkg/config"
)

func TestRetryDTOFlagsExactlyOnceRoutes(t *testing.T) {
	flagged := dlq.ActionReport{Kind: dlq.ActionRetry, Delivery: dlq.RetryDelivery{Mode: config.DeliveryModeExactlyOnce, DeliveredAtLeastOnce: true}}
	body, _ := json.Marshal(ReportDTOs{}.Retry(flagged))
	if !strings.Contains(string(body), `"deliveryMode":"exactly_once","deliveredAtLeastOnce":true`) {
		t.Fatalf("flagged %s", body)
	}
	plain := dlq.ActionReport{Kind: dlq.ActionRetry, Delivery: dlq.RetryDelivery{Mode: config.DeliveryModeAtLeastOnce}}
	body, _ = json.Marshal(ReportDTOs{}.Retry(plain))
	if !strings.Contains(string(body), `"deliveredAtLeastOnce":false`) {
		t.Fatalf("plain %s", body)
	}
}
