package httpapi

const (
	HeaderActor            = "X-Actor"
	HeaderRequestID        = "X-Request-ID"
	HeaderExpectedRevision = "X-Expected-Revision"
)

type RequestContext struct {
	Actor            string `json:"actor"`
	RequestID        string `json:"request_id"`
	ExpectedRevision int    `json:"expected_revision"`
}
