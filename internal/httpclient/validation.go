// Package httpclient provides a shared HTTP client with retry logic and metrics instrumentation.
package httpclient

import (
	"sync"

	"github.com/go-playground/validator/v10"
)

var (
	validate     *validator.Validate
	validateOnce sync.Once
)

// Validator returns the shared validator instance.
func Validator() *validator.Validate {
	validateOnce.Do(func() {
		validate = validator.New(validator.WithRequiredStructEnabled())
	})
	return validate
}

// Validate validates a struct using the shared validator.
func Validate(s interface{}) error {
	return Validator().Struct(s)
}
