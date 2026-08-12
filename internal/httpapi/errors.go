package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type ErrorKind string

const (
	KindValidation  ErrorKind = "VALIDATION_ERROR"
	KindNotFound    ErrorKind = "NOT_FOUND"
	KindTimeout     ErrorKind = "UPSTREAM_TIMEOUT"
	KindUnavailable ErrorKind = "UPSTREAM_UNAVAILABLE"
	KindInternal    ErrorKind = "INTERNAL_ERROR"
)

type APIError struct {
	Kind    ErrorKind
	Message string
	Details any
	Err     error
}

func (e *APIError) Error() string { return e.Message }
func (e *APIError) Unwrap() error { return e.Err }

type errorBody struct {
	Code      ErrorKind `json:"code"`
	Message   string    `json:"message"`
	RequestID string    `json:"request_id,omitempty"`
	Details   any       `json:"details,omitempty"`
}
type errorEnvelope struct {
	Error errorBody `json:"error"`
}
type successEnvelope struct {
	Data any `json:"data"`
	Meta any `json:"meta,omitempty"`
}

func writeData(c *gin.Context, status int, data any) { c.JSON(status, successEnvelope{Data: data}) }
func writeError(c *gin.Context, err error) {
	status, kind, message, details := classifyError(err)
	c.AbortWithStatusJSON(status, errorEnvelope{Error: errorBody{Code: kind, Message: message, RequestID: requestID(c), Details: details}})
}

func classifyError(err error) (int, ErrorKind, string, any) {
	var ae *APIError
	if errors.As(err, &ae) {
		switch ae.Kind {
		case KindValidation:
			return http.StatusBadRequest, ae.Kind, ae.Message, ae.Details
		case KindNotFound:
			return http.StatusNotFound, ae.Kind, ae.Message, ae.Details
		case KindTimeout:
			return http.StatusGatewayTimeout, ae.Kind, ae.Message, ae.Details
		case KindUnavailable:
			return http.StatusBadGateway, ae.Kind, ae.Message, ae.Details
		default:
			return http.StatusInternalServerError, KindInternal, "服务内部错误", nil
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, KindTimeout, "TDX 上游请求超时", nil
	}
	var ne net.Error
	if errors.As(err, &ne) {
		if ne.Timeout() {
			return http.StatusGatewayTimeout, KindTimeout, "TDX 上游请求超时", nil
		}
		return http.StatusBadGateway, KindUnavailable, "TDX 上游不可用", nil
	}
	if err != nil && (strings.Contains(strings.ToLower(err.Error()), "connect") || strings.Contains(strings.ToLower(err.Error()), "closed")) {
		return http.StatusBadGateway, KindUnavailable, "TDX 上游不可用", nil
	}
	return http.StatusInternalServerError, KindInternal, "服务内部错误", nil
}
