// Package serdser provides small serialization and deserialization helpers for
// Gin handlers.
// It belongs to the adapter layer and converts validation and application
// errors into JSON HTTP responses without owning business logic.
package serdser

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// Bind binds and validates the incoming request with binding strategy b.
// It writes any validation or decoding error directly to the response and
// returns false in that case, so handlers should stop processing immediately.
func Bind(c *gin.Context, req any, b binding.Binding) bool {
	switch err := c.ShouldBindWith(req, b).(type) {
	case *validator.InvalidValidationError:
		c.JSON(http.StatusInternalServerError, gin.H{
			"detail": err.Error(),
		})
	case validator.ValidationErrors:
		var nameToErrs map[string][]string
		for _, ferr := range err {
			AddErr(&nameToErrs, ferr.Field(), ferr.Error())
		}
		c.JSON(http.StatusBadRequest, nameToErrs)
	default:
		if err == nil {
			return true
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"detail": err.Error(),
		})
	}
	return false
}

// AddErr appends one or more validation messages under name in errs.
// When errs is nil, the function allocates the backing map.
func AddErr(errs *map[string][]string, name string, msgs ...string) {
	if (*errs) == nil {
		*errs = make(map[string][]string)
	}
	if elist, ok := (*errs)[name]; !ok {
		(*errs)[name] = msgs
	} else {
		(*errs)[name] = append(elist, msgs...)
	}
}

// Assert appends msgs under name and returns false when ok is false.
// It is a convenience helper for building aggregated validation responses.
func Assert(errs *map[string][]string, ok bool, name string, msgs ...string) bool {
	if ok {
		return true
	}
	AddErr(errs, name, msgs...)
	return false
}

// SerErr serializes err as a JSON error response.
// Errors wrapped with cerr.Error preserve their mapped HTTP status code;
// all other errors are returned as HTTP 500 responses.
func SerErr(c *gin.Context, err error) {
	var ce *cerr.Error
	if errors.As(err, &ce) {
		c.JSON(ce.HTTPStatusCode, gin.H{
			"detail": ce.Err.Error(),
		})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"detail": err.Error(),
	})
}
