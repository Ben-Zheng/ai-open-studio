package errors

import (
	"net/http"
)

// ErrorCode 是前后端约定的,用来标识需要处理case的那些错误码
type ErrorCode string

var (
	ParamsInvalid         ErrorCode = "params.invalid"
	Internal              ErrorCode = "error.internal"
	NotSupportGPUGroup    ErrorCode = "not.support.gpu.group"
	InsufficientQuota     ErrorCode = "insufficient.quota"
	InsufficientNormalGPU ErrorCode = "reduce.gpu.charged.quota"
)

type RSError struct {
	message   string
	httpCode  int
	errorCode ErrorCode
}

func (d RSError) Error() string {
	return d.message
}

func (d RSError) Message() string {
	return d.message
}

func (d RSError) HTTPCode() int {
	return d.httpCode
}

func (d RSError) ErrorCode() ErrorCode {
	return d.errorCode
}

var (
	ErrInternal                       = RSError{httpCode: http.StatusInternalServerError, message: "internal server error", errorCode: Internal}
	ErrParamsInvalid                  = RSError{httpCode: http.StatusBadRequest, message: "params invalid", errorCode: ParamsInvalid}
	ErrInsufficientAvailableQuota     = RSError{httpCode: http.StatusBadRequest, message: "insufficient quota", errorCode: InsufficientQuota}
	ErrSubGPUMoreThanGPUQuota         = RSError{httpCode: http.StatusBadRequest, message: "sub gpu type quota is more than gpu quota"}
	ErrSubVirtGPUMoreThanVirtGPUQuota = RSError{httpCode: http.StatusBadRequest, message: "sub virtgpu type quota is more than virtgpu quota"}
	ErrNotSupportGPUGroup             = RSError{httpCode: http.StatusForbidden, message: "not support group gpu by gpu type", errorCode: NotSupportGPUGroup}
	ErrReduceChargedNormalGPUQuota    = RSError{httpCode: http.StatusBadRequest, message: "设置稀缺卡后普通卡总量将减少，请先减少GPU分配数量再进行更新", errorCode: InsufficientNormalGPU}
)
