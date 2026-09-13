package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

func Error(c *gin.Context, status int, code, message string, details map[string]any) {
	c.AbortWithStatusJSON(status, ErrorEnvelope{Error: ErrorBody{
		Code:    code,
		Message: message,
		Details: details,
	}})
}

func Ok(c *gin.Context, status int, body any) {
	if body == nil {
		c.Status(status)
		return
	}
	c.JSON(status, body)
}

func BadRequest(c *gin.Context, msg string, details map[string]any) {
	Error(c, http.StatusBadRequest, "invalid_request", msg, details)
}

func ValidationFailed(c *gin.Context, details map[string]any) {
	Error(c, http.StatusUnprocessableEntity, "validation_failed", "Validation failed", details)
}

func Unauthorized(c *gin.Context) {
	Error(c, http.StatusUnauthorized, "unauthorized", "Authentication required", nil)
}

func Forbidden(c *gin.Context) {
	Error(c, http.StatusForbidden, "forbidden", "Insufficient permissions", nil)
}

func TenantSuspended(c *gin.Context) {
	Error(c, http.StatusForbidden, "tenant_suspended", "Tenant is suspended", nil)
}

func NotFound(c *gin.Context, msg string) {
	Error(c, http.StatusNotFound, "not_found", msg, nil)
}

func Conflict(c *gin.Context, code, msg string) {
	Error(c, http.StatusConflict, code, msg, nil)
}

func Internal(c *gin.Context, err error) {
	_ = c.Error(err)
	Error(c, http.StatusInternalServerError, "internal_error", "Internal server error", nil)
}

type List[T any] struct {
	Items []T    `json:"items"`
	Total int64  `json:"total"`
	Page  int    `json:"page"`
	Size  int    `json:"size"`
}

func Paginated[T any](c *gin.Context, items []T, total int64, page, size int) {
	c.JSON(http.StatusOK, List[T]{Items: items, Total: total, Page: page, Size: size})
}