package chunked_body_limit_bypass_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/benzhi/relay-survey/internal/httpapi"
	"github.com/benzhi/relay-survey/internal/service"
	"github.com/benzhi/relay-survey/internal/storage"
)

func TestChunkedBodyCannotBypassLimit(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httpapi.New(service.New(store))
	now := time.Now().UTC()
	payload := strings.Repeat(" ", (1<<20)+1) + `{"observation_window":{"start":"` + now.Format(time.RFC3339) + `","end":"` + now.Add(time.Hour).Format(time.RFC3339) + `"},"frequency_range_hz":[100,200],"antenna_id":"ANT-1","initial_feature":"burst"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/interference-cases", strings.NewReader(payload))
	req.ContentLength = -1
	req.Header.Set(httpapi.HeaderActor, "duty:registrar")
	req.Header.Set(httpapi.HeaderRequestID, "large-chunked")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "请求体过大") {
		t.Fatalf("未知 Content-Length 的超限请求未被拒绝: status=%d body=%s", w.Code, w.Body.String())
	}
}
