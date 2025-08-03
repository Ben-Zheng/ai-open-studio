package errors

import (
	"net/http"
)

// ErrorCode 是前后端约定的,用来标识需要处理case的那些错误码
type ErrorCode string

var (
	ParamsInvalid       ErrorCode = "params.invalid"
	Internal            ErrorCode = "error.internal"
	WorkspaceStartingUp ErrorCode = "workspace.starting.up" // workspace 正在启动中
	WorkspaceExisted    ErrorCode = "workspace.existed"     // workspace 已经存在
)

type WorkspaceServiceError struct {
	message   string
	httpCode  int
	errorCode ErrorCode
}

func (d WorkspaceServiceError) Error() string {
	return d.message
}

func (d WorkspaceServiceError) Message() string {
	return d.message
}

func (d WorkspaceServiceError) HTTPCode() int {
	return d.httpCode
}

func (d WorkspaceServiceError) ErrorCode() ErrorCode {
	return d.errorCode
}

var (
	ErrInternal              = WorkspaceServiceError{httpCode: http.StatusInternalServerError, message: "internal server error", errorCode: Internal}
	ErrParamsInvalid         = WorkspaceServiceError{httpCode: http.StatusBadRequest, message: "params invalid", errorCode: ParamsInvalid}
	ErrorWorkspaceMemory     = WorkspaceServiceError{httpCode: http.StatusBadRequest, message: "ws memory too small", errorCode: ParamsInvalid}
	ErrNotFound              = WorkspaceServiceError{httpCode: http.StatusNotFound, message: "not found"}
	ErrorWorkspaceStartingUp = WorkspaceServiceError{httpCode: http.StatusBadRequest, errorCode: WorkspaceStartingUp, message: "workspace starting up"}
	ErrorWorkspaceExisted    = WorkspaceServiceError{httpCode: http.StatusBadRequest, errorCode: WorkspaceExisted, message: "workspace has existed"}
)
