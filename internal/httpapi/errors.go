package httpapi

import (
	"errors"

	"github.com/benzhi/relay-survey/internal/casework"
)

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func errorBody(err error) ErrorBody {
	var business *casework.Error
	if errors.As(err, &business) {
		return ErrorBody{Code: business.Code, Message: business.Message, Details: business.Details}
	}
	return ErrorBody{Code: "internal_error", Message: "服务处理失败"}
}
