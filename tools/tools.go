//go:build tools

// Package tools tracks build-time tool dependencies in go.mod so `go mod tidy`
// does not remove them. It is never compiled into the application.
package tools

import (
	_ "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen"
)
