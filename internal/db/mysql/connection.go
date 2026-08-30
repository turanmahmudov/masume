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

// mysqlConnectTimeout is how long one attempt at a connection may take.
const mysqlConnectTimeout = 15 * time.Second

// mysqlTLSName is the name this client registers its own TLS settings under, because
// the driver takes a name rather than a config.
const mysqlTLSName = "masume"

// droppedLog takes what the driver would otherwise print. The screen is drawn over the whole
// terminal, so a line written to it scrolls the frame and stands on a row of its own until the
// next redraw. A killed statement leaves the connection out of step, and the driver reports
// that, so this is not a rare case.
type droppedLog struct{}

func (droppedLog) Print(...any) {}

func init() {
	_ = driver.SetLogger(droppedLog{})
}

// resolveMysqlTLS returns what the profile asks of the connection. A profile with no
// mode encrypts where the server offers it, which is what `prefer` means and what a
// MySQL client of this version does.
func resolveMysqlTLS(profile cfg.Profile) (string, error) {
	policy := core.ResolveSSLPolicy(profile.SSLMode)
	switch policy {
	case core.PolicyOff:
		return "false", nil
	case core.PolicyUnset, core.PolicyPrefer:
		// The driver encrypts where the server offers TLS, and connects in the clear
		// where it does not, so a server without TLS still opens.
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
	// `require` encrypts and checks nothing, as a MySQL client reads it.
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
	// A buffer of several statements is one read, so the driver has to allow them.
	config.MultiStatements = true
	// A MySQL date has no zone, so the driver would read it as local time and show a
	// value the server never stored.
	config.ParseTime = false
	config.InterpolateParams = false
	return config.FormatDSN(), nil
}

// mysqlSession is one session on a MySQL-protocol server, built from its engine entry

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
