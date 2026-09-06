package postgres

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// postgresConnectTimeout is the connection time limit.
const postgresConnectTimeout = 15 * time.Second

// buildPostgresTLS returns TLS settings and permission to retry without encryption. Unset and prefer modes permit unencrypted fallback.
func buildPostgresTLS(profile cfg.Profile) (*tls.Config, bool) {
	switch core.ResolveSSLPolicy(profile.SSLMode) {
	case core.PolicyOff:
		return nil, false
	case core.PolicyVerifyFull:
		return &tls.Config{ServerName: profile.Host, MinVersion: tls.VersionTLS12}, false
	case core.PolicyVerifyCa:
		return db.BuildAuthorityOnlyTLS(), false
	case core.PolicyEncryptOnly:
		// Require mode encrypts without certificate verification or unencrypted fallback.
		return &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}, false
	}
	return &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}, true
}

func buildPostgresConfig(profile cfg.Profile, password string) *pgx.ConnConfig {
	config, err := pgx.ParseConfig("")
	if err != nil {
		config = &pgx.ConnConfig{}
	}
	config.Host = profile.Host
	config.Port = uint16(profile.Port)
	config.Database = profile.Database
	config.User = profile.User
	config.Password = password
	config.ConnectTimeout = postgresConnectTimeout
	if config.RuntimeParams == nil {
		config.RuntimeParams = map[string]string{}
	}
	config.RuntimeParams["application_name"] = "masume"
	// PostgreSQL also enforces the statement time limit on the server.
	if profile.StatementTimeout > 0 {
		config.RuntimeParams["statement_timeout"] =
			strconv.FormatInt(profile.StatementTimeout.Milliseconds(), 10)
	}

	tlsConfig, mayFallBack := buildPostgresTLS(profile)
	config.TLSConfig = tlsConfig
	// Unset and prefer modes permit a retry without TLS.
	if tlsConfig != nil && mayFallBack {
		config.Fallbacks = []*pgconn.FallbackConfig{
			{Host: profile.Host, Port: uint16(profile.Port), TLSConfig: nil},
		}
	}
	// Proxies can lack named prepared statement support. Exec mode avoids the statement cache.
	config.DefaultQueryExecMode = pgx.QueryExecModeExec
	return config
}

func openPostgresConnection(
	ctx context.Context, profile cfg.Profile, password string,
) (*pgx.Conn, error) {
	return pgx.ConnectConfig(ctx, buildPostgresConfig(profile, password))
}

// keepJSONFieldOrder returns raw JSON bytes from the driver. The default map decoder loses field order.
func keepJSONFieldOrder(connection *pgx.Conn) {
	unmarshal := func(data []byte, target any) error {
		held, isAny := target.(*any)
		if !isAny {
			return json.Unmarshal(data, target)
		}
		// The driver reuses its input buffer; the result requires a copy.
		*held = json.RawMessage(bytes.Clone(data))
		return nil
	}
	types := connection.TypeMap()
	types.RegisterType(&pgtype.Type{
		Name: "json", OID: pgtype.JSONOID,
		Codec: &pgtype.JSONCodec{Marshal: json.Marshal, Unmarshal: unmarshal},
	})
	types.RegisterType(&pgtype.Type{
		Name: "jsonb", OID: pgtype.JSONBOID,
		Codec: &pgtype.JSONBCodec{Marshal: json.Marshal, Unmarshal: unmarshal},
	})
}

// readTypeNames returns server type names by OID, including custom enums, domains, and composites.
func readTypeNames(ctx context.Context, connection *pgx.Conn) (map[uint32]string, error) {
	rows, err := connection.Query(ctx, "select oid, typname from pg_type")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := map[uint32]string{}
	for rows.Next() {
		var oid uint32
		var name string
		if scanErr := rows.Scan(&oid, &name); scanErr != nil {
			return nil, scanErr
		}
		names[oid] = name
	}
	return names, rows.Err()
}
