package model

// Result 统一响应结构
type Result[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data,omitempty"`
}

// Success 成功响应
func Success[T any](data T) Result[T] {
	return Result[T]{
		Code:    0,
		Message: "success",
		Data:    data,
	}
}

// Error 错误响应
func Error[T any](message string) Result[T] {
	return Result[T]{
		Code:    -1,
		Message: message,
	}
}
