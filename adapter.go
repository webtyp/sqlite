package sqlite

import (
	"database/sql"
	"strings"

	"webtyp.com/ddl"
	"webtyp.com/fmt"
	"webtyp.com/sqlt"
	"webtyp.com/storage"

	_ "modernc.org/sqlite" // SQLite driver
)

// Open creates a new sqlite connection and returns it as a storage.Conn. No registry, no init() —
// construct explicitly: conn, err := sqlite.Open(dsn); d := orm.New(conn) (if you want the
// ergonomic layer) or use conn directly (e.g. from ddl.New(conn, ddlCompiler)).
func Open(dsn string) (storage.Conn, error) {
	raw, err := sql.Open("sqlite", normalizeDSN(dsn))
	if err != nil {
		return nil, fmt.Errf("failed to open sqlite database: %v", err)
	}
	if err := raw.Ping(); err != nil {
		raw.Close()
		return nil, fmt.Errf("failed to ping sqlite database: %v", err)
	}

	// SQLite does not support concurrent writers. In-memory databases (:memory:)
	// are per-connection — each new connection sees an empty database. Limiting
	// to a single connection prevents both "database is locked" and
	// "no such table" errors when multiple goroutines share the same conn.
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)

	return &sqliteConn{db: raw, compiler: sqlt.NewCompiler()}, nil
}

// DDLCompiler returns the ddl.Compiler half of the connection's compiler, for callers wiring
// up ddl.New(conn, sqlite.DDLCompiler(conn)).
func DDLCompiler(conn storage.Conn) (ddl.Compiler, bool) {
	c, ok := conn.(ddl.Compiler)
	return c, ok
}

// GetSqlDB returns the underlying *sql.DB, for callers that need to drop to raw SQL. Returns
// (nil, false) if conn isn't a *sqliteConn (e.g. it's storage/mem or another backend).
func GetSqlDB(conn storage.Conn) (*sql.DB, bool) {
	c, ok := conn.(*sqliteConn)
	if !ok {
		return nil, false
	}
	return c.db, true
}

func normalizeDSN(dsn string) string {
	if strings.HasPrefix(dsn, "sqlite://") {
		return strings.TrimPrefix(dsn, "sqlite://")
	}
	if strings.HasPrefix(dsn, "sqlite:") {
		return strings.TrimPrefix(dsn, "sqlite:")
	}
	return dsn
}
