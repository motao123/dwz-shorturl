package pkg

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	CodeSuccess       = 0
	CodeBadRequest    = 40000
	CodeUnauthorized  = 40100
	CodeForbidden     = 40300
	CodeNotFound      = 40400
	CodeInternalError = 50000
	CodeRateLimit     = 42900
	CodeValidation    = 42200
)

type Response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

type PaginatedData struct {
	List    interface{} `json:"list"`
	Total   int64       `json:"total"`
	Page    int         `json:"page"`
	PerPage int         `json:"per_page"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code: CodeSuccess,
		Msg:  "success",
		Data: data,
	})
}

func Fail(c *gin.Context, httpStatus, code int, msg string) {
	c.JSON(httpStatus, Response{
		Code: code,
		Msg:  msg,
	})
}

// FailWithData is like Fail but carries extra payload (e.g. totp_required).
func FailWithData(c *gin.Context, httpStatus, code int, msg string, data interface{}) {
	c.JSON(httpStatus, Response{
		Code: code,
		Msg:  msg,
		Data: data,
	})
}

func FailWithError(c *gin.Context, code int, msg string) {
	httpStatus := http.StatusOK
	switch {
	case code >= 50000:
		httpStatus = http.StatusInternalServerError
	case code >= 42900 && code < 43000:
		httpStatus = http.StatusTooManyRequests
	case code >= 40400 && code < 40500:
		httpStatus = http.StatusNotFound
	case code >= 40300 && code < 40400:
		httpStatus = http.StatusForbidden
	case code >= 40100 && code < 40200:
		httpStatus = http.StatusUnauthorized
	case code >= 40000 && code < 40100:
		httpStatus = http.StatusBadRequest
	}
	c.JSON(httpStatus, Response{
		Code: code,
		Msg:  msg,
	})
}

func Paginated(c *gin.Context, list interface{}, total int64, page, perPage int) {
	c.JSON(http.StatusOK, Response{
		Code: CodeSuccess,
		Msg:  "success",
		Data: PaginatedData{
			List:    list,
			Total:   total,
			Page:    page,
			PerPage: perPage,
		},
	})
}
