package casework

import "fmt"

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Message }
func Invalid(msg string) error { return &Error{Code: "invalid_request", Message: msg} }
func InvalidDetails(msg string, details any) error {
	return &Error{Code: "invalid_request", Message: msg, Details: details}
}
func Conflict(msg string) error { return &Error{Code: "revision_conflict", Message: msg} }
func ResourceBusy(details any) error {
	return &Error{Code: "resource_conflict", Message: "调查设备在计划窗口内已被占用", Details: details}
}
func Forbidden(msg string) error { return &Error{Code: "forbidden", Message: msg} }
func Terminal(msg string) error  { return &Error{Code: "terminal_state", Message: msg} }
func NotFound(id string) error {
	return &Error{Code: "not_found", Message: fmt.Sprintf("案件 %s 不存在", id)}
}
