// Command masume is a terminal database client for PostgreSQL, MySQL, SQLite, MongoDB, and protocol-compatible servers.
package main

import (
	"os"

	"github.com/turanmahmudov/masume/internal/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:])) }
