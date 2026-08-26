package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/benzhi/relay-survey/internal/httpapi"
	"github.com/benzhi/relay-survey/internal/service"
	"github.com/benzhi/relay-survey/internal/storage"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "", "监听地址")
	self := flag.Bool("selfcheck", false, "运行闭环自检")
	data := flag.String("data-dir", "./data", "数据目录")
	flag.Parse()
	a := *addr
	if a == "" {
		if p := os.Getenv("PORT"); p != "" {
			a = "127.0.0.1:" + p
		} else {
			a = "127.0.0.1:19081"
		}
	}
	if *self {
		if err := runSelf(a); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	r, err := storage.Open(*data)
	if err != nil {
		panic(err)
	}
	srv := httpapi.New(service.New(r))
	go srv.ListenAndServe(a)
	waitSignal(srv)
}
func waitSignal(s *httpapi.Server) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(ch)
	<-ch
	s.Shutdown()
}
func runSelf(addr string) error {
	dir, err := os.MkdirTemp("", "relay-selfcheck-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	r, _ := storage.Open(filepath.Clean(dir))
	srv := httpapi.New(service.New(r))
	go srv.ListenAndServe(addr)
	time.Sleep(80 * time.Millisecond)
	client := &http.Client{Timeout: 2 * time.Second}
	post := func(path, req, actor string, rev int) (map[string]any, error) {
		q, _ := http.NewRequest("POST", "http://"+addr+path, strings.NewReader(req))
		q.Header.Set("Content-Type", "application/json")
		q.Header.Set("X-Actor", actor)
		q.Header.Set("X-Request-ID", fmt.Sprintf("req-%d", time.Now().UnixNano()))
		if rev > 0 {
			q.Header.Set("X-Expected-Revision", fmt.Sprint(rev))
		}
		resp, e := client.Do(q)
		if e != nil {
			return nil, e
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("%s: %s", resp.Status, string(b))
		}
		var m map[string]any
		json.Unmarshal(b, &m)
		return m, nil
	}
	now := time.Now().UTC()
	w := fmt.Sprintf(`{"observation_window":{"start":%q,"end":%q},"frequency_range_hz":[1400000000,1410000000],"antenna_id":"ANT-1","initial_feature":"burst","association_disposition":"INDEPENDENT"}`, now.Add(-time.Hour).Format(time.RFC3339), now.Add(3*time.Hour).Format(time.RFC3339))
	c, e := post("/api/v1/interference-cases", w, "duty:registrar", 0)
	if e != nil {
		return e
	}
	id := c["id"].(string)
	rev := int(c["revision"].(float64))
	tri := `{"affected_observations":6,"occupied_bandwidth_hz":2000000,"persistence_minutes":90,"rationale":"影响主观测"}`
	c, e = post("/api/v1/interference-cases/"+id+"/triage", tri, "duty:leader", rev)
	if e != nil {
		return e
	}
	rev = int(c["revision"].(float64))
	plan := fmt.Sprintf(`{"measurement_sites":["site-a"],"equipment_ids":["spec-1"],"time_window":{"start":%q,"end":%q},"owner":"eng","stop_conditions":["noise<10"]}`, now.Format(time.RFC3339), now.Add(2*time.Hour).Format(time.RFC3339))
	c, e = post("/api/v1/interference-cases/"+id+"/plan", plan, "engineer:planner", rev)
	if e != nil {
		return e
	}
	rev = int(c["revision"].(float64))
	planMap := c["plan"].(map[string]any)
	items := planMap["plan_items"].([]any)
	planItemID := items[0].(map[string]any)["plan_item_id"].(string)
	ev := fmt.Sprintf(`{"evidence":[{"plan_item_id":%q,"kind":"spectrum","captured_at":%q,"source_device":"spec-1","content_hash":"%s","metrics":{"noise":5}},{"plan_item_id":%q,"kind":"device_reading","captured_at":%q,"source_device":"spec-1","content_hash":"%s","metrics":{"noise":5}},{"plan_item_id":%q,"kind":"field_observation","captured_at":%q,"source_device":"spec-1","content_hash":"%s","notes":"现场正常"}]}`, planItemID, now.Add(10*time.Minute).Format(time.RFC3339), strings.Repeat("1", 64), planItemID, now.Add(10*time.Minute).Format(time.RFC3339), strings.Repeat("2", 64), planItemID, now.Add(10*time.Minute).Format(time.RFC3339), strings.Repeat("3", 64))
	c, e = post("/api/v1/interference-cases/"+id+"/evidence", ev, "engineer:collector", rev)
	if e != nil {
		return e
	}
	rev = int(c["revision"].(float64))
	h := `{"action":"REGISTER","candidate_source":"TX-9"}`
	c, e = post("/api/v1/interference-cases/"+id+"/hypothesis", h, "investigator:analyst", rev)
	if e != nil {
		return e
	}
	rev = int(c["revision"].(float64))
	candidates := c["source_candidates"].([]any)
	candidateID := candidates[0].(map[string]any)["id"].(string)
	for i, start := range []time.Duration{20 * time.Minute, 35 * time.Minute} {
		test := fmt.Sprintf(`{"action":"ADD_TEST","candidate_id":%q,"test":{"window":{"start":%q,"end":%q},"baseline_metrics":{"power":10},"active_metrics":{"power":1}}}`, candidateID, now.Add(start).Format(time.RFC3339), now.Add(start+10*time.Minute).Format(time.RFC3339))
		c, e = post("/api/v1/interference-cases/"+id+"/hypothesis", test, "investigator:analyst", rev)
		if e != nil {
			return fmt.Errorf("来源测试 %d: %w", i+1, e)
		}
		rev = int(c["revision"].(float64))
	}
	confirm := fmt.Sprintf(`{"action":"CONFIRM","candidate_id":%q,"exclusion_notes":"其他已知来源均已排除"}`, candidateID)
	c, e = post("/api/v1/interference-cases/"+id+"/hypothesis", confirm, "investigator:analyst", rev)
	if e != nil {
		return e
	}
	rev = int(c["revision"].(float64))
	m := fmt.Sprintf(`{"measure_type":"屏蔽","measure_description":"加装屏蔽","implemented_at":%q}`, now.Add(50*time.Minute).Format(time.RFC3339))
	c, e = post("/api/v1/interference-cases/"+id+"/mitigation", m, "engineer:implementer", rev)
	if e != nil {
		return e
	}
	rev = int(c["revision"].(float64))
	attempts := c["mitigation_attempts"].([]any)
	attemptID := attempts[len(attempts)-1].(map[string]any)["attempt_id"].(string)
	v := fmt.Sprintf(`{"attempt_id":%q,"verification_window":{"start":%q,"end":%q},"thresholds":{"noise":10},"observed_metrics":{"noise":3}}`, attemptID, now.Add(60*time.Minute).Format(time.RFC3339), now.Add(70*time.Minute).Format(time.RFC3339))
	c, e = post("/api/v1/interference-cases/"+id+"/verification", v, "reviewer:verifier", rev)
	if e != nil {
		return e
	}
	rev = int(c["revision"].(float64))
	c, e = post("/api/v1/interference-cases/"+id+"/close", `{}`, "reviewer:closer", rev)
	if e != nil {
		return e
	}
	if c["state"] != "CLOSED" {
		return fmt.Errorf("未关闭")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return srv.ShutdownWithContext(ctx)
}
