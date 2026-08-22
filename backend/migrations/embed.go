// Package migrations exposes the plain SQL migration files as an embedded
// filesystem for the database package and Goose.
package migrations

import "embed"

// FS contains the SQL migrations applied at application startup.
//
//go:embed *.sql
var FS embed.FS
