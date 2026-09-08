package sqlserver

import (
	"crypto/tls"
	"database/sql"
	"time"

	mssql "github.com/microsoft/go-mssqldb"
	"github.com/microsoft/go-mssqldb/msdsn"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// sqlserverConnectTimeout is the connection time limit.
const sqlserverConnectTimeout = 15 * time.Second

// applicationName is the name this client sends with the connection.
const applicationName = "masume"

// buildEncryption returns the encryption of the connection with the TLS settings it uses.
// Unset and prefer modes encrypt the login only.
func buildEncryption(profile cfg.Profile) (msdsn.Encryption, *tls.Config) {
	insecure := &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}
	switch core.ResolveSSLPolicy(profile.SSLMode) {
	case core.PolicyOff:
		return msdsn.EncryptionDisabled, nil
	case core.PolicyVerifyFull:
		return msdsn.EncryptionRequired,
			&tls.Config{ServerName: profile.Host, MinVersion: tls.VersionTLS12}
	case core.PolicyVerifyCa:
		return msdsn.EncryptionRequired, db.BuildAuthorityOnlyTLS()
	case core.PolicyEncryptOnly:
		return msdsn.EncryptionRequired, insecure
	}
	return msdsn.EncryptionOff, insecure
}

// buildSqlserverConfig returns the connection as the driver takes it.
func buildSqlserverConfig(profile cfg.Profile, password string) msdsn.Config {
	encryption, tlsConfig := buildEncryption(profile)
	return msdsn.Config{
		Host: profile.Host, Port: uint64(profile.Port), Database: profile.Database,
		User: profile.User, Password: password,
		Encryption: encryption, TLSConfig: tlsConfig,
		TrustServerCertificate: tlsConfig != nil && tlsConfig.InsecureSkipVerify,
		AppName:                applicationName,
		DialTimeout:            sqlserverConnectTimeout,
		// The driver dials the protocols of this list, and it dials none without one.
		Protocols:  []string{"tcp"},
		Parameters: map[string]string{},
	}
}

// openSqlserverPool opens a pool limited to one connection.
func openSqlserverPool(profile cfg.Profile, password string) *sql.DB {
	pool := sql.OpenDB(mssql.NewConnectorConfig(buildSqlserverConfig(profile, password)))
	pool.SetMaxOpenConns(1)
	return pool
}
