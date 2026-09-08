package clickhouse

import (
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	driver "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// clickhouseConnectTimeout is the connection time limit.
const clickhouseConnectTimeout = 15 * time.Second

// clientName is the name this client sends, which the activity list of the server shows.
const clientName = "masume"

// buildTLS returns the TLS settings of the connection. The native protocol does not
// negotiate, so an unset mode and a preferred mode both connect without encryption.
func buildTLS(profile cfg.Profile) *tls.Config {
	switch core.ResolveSSLPolicy(profile.SSLMode) {
	case core.PolicyVerifyFull:
		return &tls.Config{ServerName: profile.Host, MinVersion: tls.VersionTLS12}
	case core.PolicyVerifyCa:
		return db.BuildAuthorityOnlyTLS()
	case core.PolicyEncryptOnly:
		return &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}
	}
	return nil
}

// buildOptions returns the connection as the driver takes it. A read-only session sends no
// setting of its own, because the server refuses to change one.
func buildOptions(profile cfg.Profile, password string) *driver.Options {
	settings := driver.Settings{}
	if profile.AccessMode != cfg.AccessReadOnly {
		// A staged edit is a mutation, and this makes the server finish it before it
		// answers, so the grid reads the row back as it now stands.
		settings["mutations_sync"] = 1
	}
	return &driver.Options{
		Addr: []string{fmt.Sprintf("%s:%d", profile.Host, profile.Port)},
		Auth: driver.Auth{
			Database: profile.Database, Username: profile.User, Password: password,
		},
		Settings:    settings,
		TLS:         buildTLS(profile),
		DialTimeout: clickhouseConnectTimeout,
		ClientInfo:  driver.ClientInfo{Products: []struct{ Name, Version string }{{Name: clientName}}},
	}
}

// openClickhousePool opens a pool limited to one connection.
func openClickhousePool(profile cfg.Profile, password string) *sql.DB {
	pool := driver.OpenDB(buildOptions(profile, password))
	pool.SetMaxOpenConns(1)
	return pool
}

// buildQueryID returns the id this client gives one statement, so a second connection can
// stop it.
func buildQueryID() string {
	held := make([]byte, 8)
	if _, err := rand.Read(held); err != nil {
		return ""
	}
	return clientName + "-" + hex.EncodeToString(held)
}
