// Package response provides helpers for the consistent API envelope used across
// the backend: {"data":…, "message":…} on success and {"error":{…}} on failure.
package response

import "github.com/gin-gonic/gin"

// Error codes returned to clients. Keep these stable; clients may branch on them.
const (
	CodeValidation   = "VALIDATION_ERROR"
	CodeUnauthorized = "UNAUTHORIZED"
	CodeForbidden    = "FORBIDDEN"
	CodeNotFound     = "NOT_FOUND"
	CodeConflict     = "CONFLICT"
	CodeInternal     = "INTERNAL_ERROR"
)

// Success writes a standard success envelope.
func Success(c *gin.Context, status int, data any, message string) {
	if message == "" {
		message = "Success"
	}
	c.JSON(status, gin.H{"data": data, "message": message})
}

// Error writes a standard error envelope. fields may be nil.
func Error(c *gin.Context, status int, code, message string, fields map[string]string) {
	body := gin.H{"code": code, "message": message}
	if fields != nil {
		body["fields"] = fields
	}
	c.JSON(status, gin.H{"error": body})
}
