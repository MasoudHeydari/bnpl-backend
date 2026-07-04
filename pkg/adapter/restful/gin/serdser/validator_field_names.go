package serdser

import (
	"reflect"
	"strings"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

func init() {
	configureJSONValidationFieldNames()
}

// configureJSONValidationFieldNames makes validator.FieldError.Field report the
// public JSON wire name (snake_case) instead of the Go struct field name.
//
// Public validation details.fields must match request JSON keys so clients can
// correct the payload without reverse-engineering Go identifiers.
func configureJSONValidationFieldNames() {
	engine, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return
	}

	engine.RegisterTagNameFunc(jsonValidationFieldName)
}

func jsonValidationFieldName(fld reflect.StructField) string {
	name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
	if name == "" || name == "-" {
		return fld.Name
	}

	return name
}
