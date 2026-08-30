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

// postgresConnectTimeout is how long one attempt at a connection may take.
const postgresConnectTimeout = 15 * time.Second

// buildPostgresTLS returns the TLS settings a policy asks for, and nothing where the
// connection stays in the clear. The modes follow libpq: only the two verifying modes
// check the certificate, which is what a profile asks for when it names one. The second
// answer says whether the connection may fall back to the clear where the server refuses
// TLS, which is what `prefer` means and what a profile that names no mode gets.
func buildPostgresTLS(profile cfg.Profile) (*tls.Config, bool) {
	switch core.ResolveSSLPolicy(profile.SSLMode) {
	case core.PolicyOff:
		return nil, false
	case core.PolicyVerifyFull:
		return &tls.Config{ServerName: profile.Host, MinVersion: tls.VersionTLS12}, false
	case core.PolicyVerifyCa:
		return db.BuildAuthorityOnlyTLS(), false
	case core.PolicyEncryptOnly:
		// `require` encrypts and checks nothing, as libpq reads it, and it never falls
		// back to the clear.
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
	// The server holds the limit too, so a statement that passes it is stopped where it
	// runs and not only in this client.
	if profile.StatementTimeout > 0 {
		config.RuntimeParams["statement_timeout"] =
			strconv.FormatInt(profile.StatementTimeout.Milliseconds(), 10)
	}

	tlsConfig, mayFallBack := buildPostgresTLS(profile)
	config.TLSConfig = tlsConfig
	// A server that refuses TLS is tried again in the clear, which is what `prefer` and
	// a profile that names no mode ask for.
	if tlsConfig != nil && mayFallBack {
		config.Fallbacks = []*pgconn.FallbackConfig{
			{Host: profile.Host, Port: uint16(profile.Port), TLSConfig: nil},
		}
	}
	// A statement is never cached, because the client sends each one once and a proxy
	// in front of the server can hold no prepared name.
	config.DefaultQueryExecMode = pgx.QueryExecModeExec
	return config
}

func openPostgresConnection(
	ctx context.Context, profile cfg.Profile, password string,
) (*pgx.Conn, error) {
	return pgx.ConnectConfig(ctx, buildPostgresConfig(profile, password))
}

// keepJSONFieldOrder makes the driver hand a JSON value over as the bytes the server sent.
// The codec of the driver reads one into a map, and a map has no order, so the fields of a
// value would be written back in name order rather than in the order they are stored in.
func keepJSONFieldOrder(connection *pgx.Conn) {
	unmarshal := func(data []byte, target any) error {
		held, isAny := target.(*any)
		if !isAny {
			return json.Unmarshal(data, target)
		}
		// The driver reuses the buffer it read into, so the bytes are kept here.
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

// readTypeNames reads the name of every type the server holds, so a result column carries
// the name of an enum, a domain or a composite the driver ships no entry for.
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
