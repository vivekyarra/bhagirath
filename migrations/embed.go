// Package migrations embeds the ordered SQL schema migrations.
package migrations

import "embed"

// FS holds every NNNN_name.sql file, applied in lexical order.
//
//go:embed *.sql
var FS embed.FS
