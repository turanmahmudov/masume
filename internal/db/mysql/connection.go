package mysql

import (
	"crypto/tls"
	"database/sql"
	"fmt"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// mysqlConnectTimeout is the connection time limit.
const mysqlConnectTimeout = 15 * time.Second

// mysqlTLSName is the registered TLS configuration name. The driver requires a name for custom TLS settings.
const mysqlTLSName = "masume"

// droppedLog discards direct driver output to the terminal.
type droppedLog struct{}

func (droppedLog) Print(...any) {}

func init() {
	_ = driver.SetLogger(droppedLog{})
}

// resolveMysqlTLS returns the driver TLS setting. Unset and prefer modes allow unencrypted connections.
func resolveMysqlTLS(profile cfg.Profile) (string, error) {
	policy := core.ResolveSSLPolicy(profile.SSLMode)
	switch policy {
	case core.PolicyOff:
		return "false", nil
	case core.PolicyUnset, core.PolicyPrefer:
		// Preferred mode uses TLS if available and otherwise connects without encryption.
		return "preferred", nil
	case core.PolicyVerifyFull:
		return "true", nil
	case core.PolicyVerifyCa:
		name := mysqlTLSName + "-verify-ca"
		if err := driver.RegisterTLSConfig(name, db.BuildAuthorityOnlyTLS()); err != nil {
			return "", err
		}
		return name, nil
	}
	// Require mode encrypts without certificate verification.
	name := mysqlTLSName + "-skip-verify"
	if err := driver.RegisterTLSConfig(
		name, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}); err != nil {
		return "", err
	}
	return name, nil
}

func buildMysqlDsn(profile cfg.Profile, password string) (string, error) {
	tlsName, err := resolveMysqlTLS(profile)
	if err != nil {
		return "", err
	}

	config := driver.NewConfig()
	config.User = profile.User
	config.Passwd = password
	config.Net = "tcp"
	config.Addr = fmt.Sprintf("%s:%d", profile.Host, profile.Port)
	config.DBName = profile.Database
	config.Timeout = mysqlConnectTimeout
	config.TLSConfig = tlsName
	// Query buffers can contain multiple statements.
	config.MultiStatements = true
	// MySQL dates have no time zone. Text preserves their stored values.
	config.ParseTime = false
	config.InterpolateParams = false
	return config.FormatDSN(), nil
}

// openMysqlPool opens a pool limited to one connection.

func openMysqlPool(profile cfg.Profile, password string) (*sql.DB, error) {
	dsn, err := buildMysqlDsn(profile, password)
	if err != nil {
		return nil, err
	}
	pool, openErr := sql.Open("mysql", dsn)
	if openErr != nil {
		return nil, openErr
	}
	pool.SetMaxOpenConns(1)
	return pool, nil
}
