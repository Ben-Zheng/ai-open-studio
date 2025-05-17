package errors

import "net/http"

// ErrorCode 是前后端约定的,用来标识需要处理case的那些错误码
type ErrorCode string

const (
	CodeServerInternalError  ErrorCode = "1001"
	CodeKFServingError       ErrorCode = "1002"
	CodeModelHubNotAvailable ErrorCode = "1003"

	CodeDatabaseNotFoundError      ErrorCode = "2001"
	CodeDatabaseDuplicatedKeyError ErrorCode = "2002"

	CodeInsufficientParamError ErrorCode = "3001"
	CodeInvalidParamError      ErrorCode = "3003"
	CodeInvalidStageChange     ErrorCode = "3004"
	CodeInvalidInstanceEnv     ErrorCode = "3005"
	CodeServiceIsNotNormal     ErrorCode = "3006"

	CodeQuotaInsufficientError ErrorCode = "4001"

	CodeConversationRecordNotFoundError ErrorCode = "5001"
	CodeConversationRecordMaxToken      ErrorCode = "5002"
)

type InferenceError struct {
	message   string
	httpCode  int
	errorCode ErrorCode
}

func (d InferenceError) Error() string {
	return d.message
}

func (d InferenceError) Message() string {
	return d.message
}

func (d InferenceError) HTTPCode() int {
	return d.httpCode
}

func (d InferenceError) ErrorCode() ErrorCode {
	return d.errorCode
}

func NewInferenceError(message string, httpCode int, errorCode ErrorCode) InferenceError {
	return InferenceError{message: message, httpCode: httpCode, errorCode: errorCode}
}

var (
	ErrInternal                   = InferenceError{httpCode: http.StatusInternalServerError, message: "internal server error", errorCode: CodeServerInternalError}
	ErrKserve                     = InferenceError{httpCode: http.StatusInternalServerError, message: "deploy inference service error", errorCode: CodeKFServingError}
	ErrModelHub                   = InferenceError{httpCode: http.StatusInternalServerError, message: "modelhub service is not available", errorCode: CodeModelHubNotAvailable}
	ErrDatabaseNotFound           = InferenceError{httpCode: http.StatusNotFound, message: "document not found", errorCode: CodeDatabaseNotFoundError}
	ErrDatabaseDuplicatedKey      = InferenceError{httpCode: http.StatusBadRequest, message: "duplicate revision error", errorCode: CodeDatabaseDuplicatedKeyError}
	ErrInvalidParam               = InferenceError{httpCode: http.StatusBadRequest, message: "invalid request param", errorCode: CodeInvalidParamError}
	ErrInvalidEnv                 = InferenceError{httpCode: http.StatusBadRequest, message: "invalid instance env", errorCode: CodeInvalidInstanceEnv}
	ErrInvalidStateChange         = InferenceError{httpCode: http.StatusBadRequest, message: "invalid inference state change", errorCode: CodeInvalidStageChange}
	ErrConversationRecordNotFound = InferenceError{httpCode: http.StatusNotFound, message: "document not found", errorCode: CodeConversationRecordNotFoundError}
	ErrInferenceNotNormal         = InferenceError{httpCode: http.StatusBadRequest, message: "service not available", errorCode: CodeServiceIsNotNormal}
	ErrInferenceMaxToken          = InferenceError{httpCode: http.StatusBadRequest, message: "reach max token", errorCode: CodeConversationRecordMaxToken}
)
