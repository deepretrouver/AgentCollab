package migrations

import "embed"

// Files contains the immutable, ordered Hub database migrations.
//
//go:embed *.sql
var Files embed.FS
