package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/benzhi/relay-survey/internal/casework"
	"github.com/benzhi/relay-survey/internal/service"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	svc  *service.Service
	http *http.Server
}

type closeResult struct {
	caseValue *casework.InterferenceCase
	err       error
}

func New(svc *service.Service) *Server {
	s := &Server{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", s.ready)
	mux.HandleFunc("/api/v1/interference-cases", s.collection)
	mux.HandleFunc("/api/v1/interference-cases/", s.caseRoute)
	s.http = &http.Server{Handler: logging(mux), ReadHeaderTimeout: 5 * time.Second}
	return s
}
func (s *Server) Handler() http.Handler { return s.http.Handler }
func (s *Server) ListenAndServe(addr string) error {
	s.http.Addr = addr
	return s.http.ListenAndServe()
}
func (s *Server) Shutdown() error                               { return s.http.Close() }
func (s *Server) ShutdownWithContext(ctx context.Context) error { return s.http.Shutdown(ctx) }
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	write(w, 200, map[string]string{"status": "ready"})
}
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { next.ServeHTTP(w, r) })
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func decode(r *http.Request, v any) error {
	if r.ContentLength > 1<<20 {
		return casework.Invalid("请求体过大")
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return casework.Invalid("请求格式无效: " + err.Error())
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return casework.Invalid("请求体只能包含一个 JSON 值")
	}
	return nil
}
func meta(r *http.Request) service.Meta {
	rev, _ := strconv.Atoi(r.Header.Get(HeaderExpectedRevision))
	return service.Meta{Actor: r.Header.Get(HeaderActor), RequestID: r.Header.Get(HeaderRequestID), ExpectedRevision: rev}
}
func errStatus(err error) int {
	if e, ok := err.(*casework.Error); ok {
		switch e.Code {
		case "not_found":
			return 404
		case "revision_conflict":
			return 409
		case "resource_conflict", "terminal_state":
			return 409
		case "forbidden":
			return 403
		}
	}
	return 400
}
func respond(w http.ResponseWriter, c *casework.InterferenceCase, err error) {
	if err != nil {
		write(w, errStatus(err), errorBody(err))
		return
	}
	write(w, 200, c)
}
func (s *Server) collection(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		q, err := parseCaseListQuery(r)
		if err != nil {
			write(w, 400, errorBody(err))
			return
		}
		page, err := s.svc.ListCases(q)
		if err != nil {
			write(w, errStatus(err), errorBody(err))
			return
		}
		write(w, 200, page)
		return
	}
	if r.Method != "POST" {
		write(w, 405, nil)
		return
	}
	var in service.CreateInput
	if err := decode(r, &in); err != nil {
		respond(w, nil, err)
		return
	}
	c, e := s.svc.Create(meta(r), in)
	if e != nil {
		respond(w, nil, e)
		return
	}
	write(w, 201, c)
}
func (s *Server) caseRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/interference-cases/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		write(w, 404, nil)
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == "GET" {
		v, e := s.svc.Detail(id)
		if e != nil {
			respond(w, nil, e)
		} else {
			write(w, 200, v)
		}
		return
	}
	if len(parts) == 2 && parts[1] == "audit" && r.Method == "GET" {
		from, parseErr := optionalInt(r.URL.Query().Get("from_revision"))
		pageSize, sizeErr := optionalInt(r.URL.Query().Get("page_size"))
		if parseErr != nil || sizeErr != nil {
			write(w, 400, errorBody(casework.Invalid("from_revision 或 page_size 无效")))
			return
		}
		a, e := s.svc.AuditPage(id, service.AuditQuery{FromRevision: from, EventType: r.URL.Query().Get("event_type"), PageSize: pageSize, Cursor: r.URL.Query().Get("cursor")})
		if e != nil {
			respond(w, nil, e)
		} else {
			write(w, 200, a)
		}
		return
	}
	if r.Method != "POST" || len(parts) != 2 {
		write(w, 405, nil)
		return
	}
	switch parts[1] {
	case "triage":
		var v casework.Triage
		if decode(r, &v) != nil {
			write(w, 400, map[string]string{"error": "请求格式无效"})
			return
		}
		c, e := s.svc.Triage(meta(r), id, v)
		respond(w, c, e)
	case "plan":
		var v casework.InvestigationPlan
		if decode(r, &v) != nil {
			write(w, 400, map[string]string{"error": "请求格式无效"})
			return
		}
		c, e := s.svc.Plan(meta(r), id, v)
		respond(w, c, e)
	case "evidence":
		batch, withdrawal, err := decodeEvidenceRequest(r)
		if err != nil {
			write(w, 400, errorBody(err))
			return
		}
		var c *casework.InterferenceCase
		var e error
		if withdrawal != nil {
			c, e = s.svc.WithdrawEvidence(meta(r), id, withdrawal.EvidenceIDs, withdrawal.CorrectionReason)
		} else {
			c, e = s.svc.EvidenceBatch(meta(r), id, batch)
		}
		respond(w, c, e)
	case "hypothesis":
		var v service.HypothesisCommand
		if decode(r, &v) != nil {
			write(w, 400, map[string]string{"error": "请求格式无效"})
			return
		}
		c, e := s.svc.HypothesisAction(meta(r), id, v)
		respond(w, c, e)
	case "mitigation":
		var v casework.MitigationVerification
		if decode(r, &v) != nil {
			write(w, 400, map[string]string{"error": "请求格式无效"})
			return
		}
		c, e := s.svc.Mitigation(meta(r), id, v)
		respond(w, c, e)
	case "verification":
		var v casework.MitigationVerification
		if decode(r, &v) != nil {
			write(w, 400, map[string]string{"error": "请求格式无效"})
			return
		}
		c, e := s.svc.Verification(meta(r), id, v)
		respond(w, c, e)
	case "close":
		result := make(chan closeResult, 1)
		go func() {
			c, e := s.svc.Close(meta(r), id)
			result <- closeResult{caseValue: c, err: e}
		}()
		select {
		case completed := <-result:
			respond(w, completed.caseValue, completed.err)
		case <-r.Context().Done():
			write(w, http.StatusRequestTimeout, ErrorBody{Code: "request_canceled", Message: "请求已取消"})
		}
	default:
		write(w, 404, nil)
	}
}

func optionalInt(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return 0, casework.Invalid("整数查询参数无效")
	}
	return v, nil
}

func parseCaseListQuery(r *http.Request) (service.CaseListQuery, error) {
	allowed := map[string]bool{"state": true, "severity": true, "antenna_id": true, "observation_start": true, "observation_end": true, "observed_from": true, "observed_to": true, "has_blockers": true, "sort": true, "page_size": true, "cursor": true}
	for key := range r.URL.Query() {
		if !allowed[key] {
			return service.CaseListQuery{}, casework.Invalid("包含未知查询参数: " + key)
		}
	}
	pageSize, err := optionalInt(r.URL.Query().Get("page_size"))
	if err != nil {
		return service.CaseListQuery{}, err
	}
	q := service.CaseListQuery{AntennaID: r.URL.Query().Get("antenna_id"), Sort: r.URL.Query().Get("sort"), PageSize: pageSize, Cursor: r.URL.Query().Get("cursor")}
	for _, raw := range r.URL.Query()["state"] {
		for _, value := range strings.Split(raw, ",") {
			q.States = append(q.States, casework.State(value))
		}
	}
	for _, raw := range r.URL.Query()["severity"] {
		q.Severities = append(q.Severities, strings.Split(raw, ",")...)
	}
	startRaw, err := oneTimeAlias(r, "observation_start", "observed_from")
	if err != nil {
		return service.CaseListQuery{}, err
	}
	endRaw, err := oneTimeAlias(r, "observation_end", "observed_to")
	if err != nil {
		return service.CaseListQuery{}, err
	}
	if startRaw != "" {
		start, parseErr := time.Parse(time.RFC3339, startRaw)
		if parseErr != nil {
			return service.CaseListQuery{}, casework.Invalid("观测开始时间必须使用 RFC3339")
		}
		q.ObservationStart = &start
	}
	if endRaw != "" {
		end, parseErr := time.Parse(time.RFC3339, endRaw)
		if parseErr != nil {
			return service.CaseListQuery{}, casework.Invalid("观测结束时间必须使用 RFC3339")
		}
		q.ObservationEnd = &end
	}
	if raw, exists := r.URL.Query()["has_blockers"]; exists {
		if len(raw) != 1 {
			return service.CaseListQuery{}, casework.Invalid("has_blockers 只能提供一次")
		}
		value, parseErr := strconv.ParseBool(raw[0])
		if parseErr != nil {
			return service.CaseListQuery{}, casework.Invalid("has_blockers 必须为 true 或 false")
		}
		q.HasBlockers = &value
	}
	return q, nil
}

func oneTimeAlias(r *http.Request, primary, alias string) (string, error) {
	a, b := r.URL.Query()[primary], r.URL.Query()[alias]
	if len(a) > 1 || len(b) > 1 || (len(a) == 1 && len(b) == 1) {
		return "", casework.Invalid(primary + " 查询参数重复或互相矛盾")
	}
	if len(a) == 1 {
		return a[0], nil
	}
	if len(b) == 1 {
		return b[0], nil
	}
	return "", nil
}

func decodeEvidence(r *http.Request) ([]casework.EvidenceRecord, error) {
	batch, withdrawal, err := decodeEvidenceRequest(r)
	if err != nil {
		return nil, err
	}
	if withdrawal != nil {
		return nil, casework.Invalid("撤回命令不是证据新增批次")
	}
	return batch, nil
}

type evidenceWithdrawalCommand struct {
	Action           string   `json:"action"`
	Command          string   `json:"command,omitempty"`
	EvidenceIDs      []string `json:"evidence_ids"`
	CorrectionReason string   `json:"correction_reason"`
}

func decodeEvidenceRequest(r *http.Request) ([]casework.EvidenceRecord, *evidenceWithdrawalCommand, error) {
	if r.ContentLength > 1<<20 {
		return nil, nil, casework.Invalid("请求体过大")
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return nil, nil, casework.Invalid("请求体过大")
	}
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 {
		return nil, nil, casework.Invalid("证据请求体为空")
	}
	var batch []casework.EvidenceRecord
	if trimmed[0] == '[' {
		if err := strictBytes(trimmed, &batch); err != nil {
			return nil, nil, err
		}
	} else {
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &probe); err != nil {
			return nil, nil, casework.Invalid("请求格式无效")
		}
		_, hasAction := probe["action"]
		_, hasCommand := probe["command"]
		if hasAction || hasCommand {
			var command evidenceWithdrawalCommand
			if err := strictBytes(trimmed, &command); err != nil {
				return nil, nil, err
			}
			if command.Action != "" && command.Command != "" && !strings.EqualFold(strings.TrimSpace(command.Action), strings.TrimSpace(command.Command)) {
				return nil, nil, casework.Invalid("action 与 command 互相矛盾")
			}
			action := command.Action
			if action == "" {
				action = command.Command
			}
			if strings.ToUpper(strings.TrimSpace(action)) != "WITHDRAW" {
				return nil, nil, casework.Invalid("evidence action 仅允许 WITHDRAW")
			}
			if len(command.EvidenceIDs) < 1 || len(command.EvidenceIDs) > 50 {
				return nil, nil, casework.Invalid("evidence_ids 必须包含 1 到 50 个标识")
			}
			return nil, &command, nil
		}
		if raw, ok := probe["evidence"]; ok {
			if len(probe) != 1 {
				return nil, nil, casework.Invalid("批量证据对象包含未知字段")
			}
			if err := strictBytes(raw, &batch); err != nil {
				return nil, nil, err
			}
		} else if raw, ok := probe["items"]; ok {
			if len(probe) != 1 {
				return nil, nil, casework.Invalid("批量证据对象包含未知字段")
			}
			if err := strictBytes(raw, &batch); err != nil {
				return nil, nil, err
			}
		} else {
			var one casework.EvidenceRecord
			if err := strictBytes(trimmed, &one); err != nil {
				return nil, nil, err
			}
			batch = []casework.EvidenceRecord{one}
		}
	}
	if len(batch) == 0 || len(batch) > 50 {
		return nil, nil, casework.Invalid("证据批次必须包含 1 到 50 条记录")
	}
	return batch, nil, nil
}

func strictBytes(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return casework.Invalid("请求格式无效: " + err.Error())
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return casework.Invalid("请求体只能包含一个 JSON 值")
	}
	return nil
}
