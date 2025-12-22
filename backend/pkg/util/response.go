package util

// 业务错误码
const (
	CodeSuccess     = 0
	CodeParamError  = 400
	CodeServerError = 500
	CodeSoldOut     = 1001
	CodeLimitExceed = 1002
)

// Response 统一响应结构
type Response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

// Success 成功响应
func Success(data interface{}) Response {
	return Response{Code: CodeSuccess, Msg: "success", Data: data}
}

// Error 错误响应
func Error(code int, msg string) Response {
	return Response{Code: code, Msg: msg}
}
