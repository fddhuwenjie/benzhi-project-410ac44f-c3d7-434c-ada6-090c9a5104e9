package idempotency_request_mismatch_test

import (
	"testing"
	"time"

	"github.com/benzhi/relay-survey/internal/casework"
	"github.com/benzhi/relay-survey/internal/service"
	"github.com/benzhi/relay-survey/internal/storage"
)

func TestIdempotencyKeyRejectsDifferentRequest(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(store)
	now := time.Now().UTC()
	first, err := svc.Create(service.Meta{Actor: "duty:registrar", RequestID: "shared-key"}, service.CreateInput{
		ObservationWindow: casework.ObservationWindow{Start: now, End: now.Add(time.Hour)},
		FrequencyRangeHz:  [2]float64{100, 200},
		AntennaID:         "ANT-1",
		InitialFeature:    "burst",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Create(service.Meta{Actor: "duty:other", RequestID: "shared-key"}, service.CreateInput{
		ObservationWindow: casework.ObservationWindow{Start: now.Add(2 * time.Hour), End: now.Add(3 * time.Hour)},
		FrequencyRangeHz:  [2]float64{300, 400},
		AntennaID:         "ANT-2",
		InitialFeature:    "continuous tone",
	})
	if err == nil {
		t.Fatalf("不同请求复用了旧幂等结果: first=%s second=%s antenna=%s", first.ID, second.ID, second.AntennaID)
	}
}
