package storage

import (
	"testing"
	"time"

	"github.com/benzhi/relay-survey/internal/casework"
)

func TestStoreRoundTrip(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := &casework.InterferenceCase{ID: "c", State: casework.StateDetected, Revision: 1, CreatedAt: time.Now()}
	if err := s.PutCase(c); err != nil {
		t.Fatal(err)
	}
	if got, ok := s.GetCase("c"); !ok || got.Revision != 1 {
		t.Fatal("快照读取失败")
	}
}
