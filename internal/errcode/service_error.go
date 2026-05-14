package errcode

// ServiceError 携带业务错误码，handler 层通过 errors.As 提取。
type ServiceError struct {
	Code int
}

func (e *ServiceError) Error() string { return Msg(e.Code) }

func New(code int) *ServiceError { return &ServiceError{Code: code} }
