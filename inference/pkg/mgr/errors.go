package mgr

import "net/http"

const (
	CodeServerInternalError = "1001"
	CodeKFServingError      = "1002"

	CodeDatabaseInternalError      = "2001"
	CodeDatabaseNotFoundError      = "2002"
	CodeDatabaseDuplicatedKeyError = "2003"

	CodeInsufficientParamError = "3001"
	CodeParamUnmarshalError    = "3002"
	CodeInvalidParamError      = "3003"

	CodeQuotaInsufficientError = "4001"
)

type WrapedError struct {
	HTTPCode  int
	ErrorCode string
	Message   string
}

func NewWrapedError(httpCode int, errorCode string, message string) *WrapedError {
	return &WrapedError{
		HTTPCode:  httpCode,
		ErrorCode: errorCode,
		Message:   message,
	}
}

func InternalError(err error) *WrapedError {
	return NewWrapedError(http.StatusInternalServerError, CodeServerInternalError, err.Error())
}

func KFServingError(err error) *WrapedError {
	return NewWrapedError(http.StatusInternalServerError, CodeKFServingError, err.Error())
}

func DatabaseInternalError(err error) *WrapedError {
	return NewWrapedError(http.StatusInternalServerError, CodeDatabaseInternalError, err.Error())
}

func DatabaseNotFoundError(err error) *WrapedError {
	return NewWrapedError(http.StatusBadRequest, CodeDatabaseNotFoundError, err.Error())
}

func DatabaseDuplicatedKeyError(err error) *WrapedError {
	return NewWrapedError(http.StatusBadRequest, CodeDatabaseDuplicatedKeyError, err.Error())
}

func InsufficientParamError(err error) *WrapedError {
	return NewWrapedError(http.StatusBadRequest, CodeInsufficientParamError, err.Error())
}

func ParamUnmarshalError(err error) *WrapedError {
	return NewWrapedError(http.StatusBadRequest, CodeParamUnmarshalError, err.Error())
}

func InvalidParamError(err error) *WrapedError {
	return NewWrapedError(http.StatusBadRequest, CodeInvalidParamError, err.Error())
}

func QuotaInsufficientError(err error) *WrapedError {
	return NewWrapedError(http.StatusBadRequest, CodeQuotaInsufficientError, err.Error())
}
