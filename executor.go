package sqlite

import (
	"database/sql"
	"sync"

	"webtyp.com/ddl"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/storage"
)

// sqliteConn implements storage.Conn (Executor+Compiler) plus storage.TxExecutor, ddl.TableIntrospector,
// and ddl.SchemaInspector — everything a SQL backend can offer. Renamed from sqliteExecutor:
// it's no longer just an Executor, storage.Conn requires the Compiler half too.
type sqliteConn struct {
	db       *sql.DB
	compiler storage.Compiler // sqlt.NewCompiler() — also a ddl.Compiler

	mu       sync.Mutex
	activeTx *sql.Tx
}

func (c *sqliteConn) Compile(q storage.Query, m model.Model) (storage.Plan, error) {
	return c.compiler.Compile(q, m)
}

func (c *sqliteConn) CompileDDL(s ddl.Stmt, m model.Model) (string, []any, error) {
	dc, ok := c.compiler.(ddl.Compiler)
	if !ok {
		return "", nil, fmt.Err("compiler does not support DDL")
	}
	return dc.CompileDDL(s, m)
}

func (c *sqliteConn) Exec(query string, args ...any) (storage.Result, error) {
	c.mu.Lock()
	tx := c.activeTx
	c.mu.Unlock()

	if tx != nil {
		res, err := tx.Exec(query, args...)
		if err != nil {
			return storage.Affected(0), mapExecError(err)
		}
		n, _ := res.RowsAffected()
		return storage.Affected(n), nil
	}
	res, err := c.db.Exec(query, args...)
	if err != nil {
		return storage.Affected(0), mapExecError(err)
	}
	n, _ := res.RowsAffected()
	return storage.Affected(n), nil
}

func (c *sqliteConn) QueryRow(query string, args ...any) storage.Scanner {
	c.mu.Lock()
	tx := c.activeTx
	c.mu.Unlock()

	if tx != nil {
		return &errScanner{s: tx.QueryRow(query, args...)}
	}
	return &errScanner{s: c.db.QueryRow(query, args...)}
}

func (c *sqliteConn) Query(query string, args ...any) (storage.Rows, error) {
	c.mu.Lock()
	tx := c.activeTx
	c.mu.Unlock()

	if tx != nil {
		rows, err := tx.Query(query, args...)
		if err != nil {
			return nil, err
		}
		return nullRows{rows}, nil
	}
	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return nullRows{rows}, nil
}

func (c *sqliteConn) Close() error {
	return c.db.Close()
}

func (c *sqliteConn) BeginTx() (storage.TxBoundExecutor, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	tx, err := c.db.Begin()
	if err != nil {
		return nil, err
	}
	c.activeTx = tx
	return &sqliteTxExecutor{tx: tx, conn: c}, nil
}

func (c *sqliteConn) clearActiveTx(tx *sql.Tx) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activeTx == tx {
		c.activeTx = nil
	}
}

// sqliteTxExecutor implements storage.TxBoundExecutor. It does NOT implement storage.Compiler on its
// own (compiling doesn't depend on being inside a transaction) — callers that need to compile
// while inside a Tx use the original sqliteConn's Compile.
type sqliteTxExecutor struct {
	tx   *sql.Tx
	conn *sqliteConn
}

func (e *sqliteTxExecutor) Exec(query string, args ...any) (storage.Result, error) {
	res, err := e.tx.Exec(query, args...)
	if err != nil {
		return storage.Affected(0), mapExecError(err)
	}
	n, _ := res.RowsAffected()
	return storage.Affected(n), nil
}

// SQLite's extended result codes for a repeated primary key and a repeated unique value.
const (
	sqliteConstraintPrimaryKey = 1555
	sqliteConstraintUnique     = 2067
)

// mapExecError marks a unique-key violation as storage.Duplicate and leaves any other error as the driver reported it.
func mapExecError(err error) error {
	if err == nil {
		return nil
	}
	type coder interface {
		Code() int
	}
	if c, ok := err.(coder); ok {
		code := c.Code()
		if code == sqliteConstraintPrimaryKey || code == sqliteConstraintUnique {
			return storage.Duplicate(err)
		}
	}
	return err
}

func (e *sqliteTxExecutor) QueryRow(query string, args ...any) storage.Scanner {
	return &errScanner{s: e.tx.QueryRow(query, args...)}
}

func (e *sqliteTxExecutor) Query(query string, args ...any) (storage.Rows, error) {
	rows, err := e.tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return nullRows{rows}, nil
}

func (e *sqliteTxExecutor) Commit() error {
	err := e.tx.Commit()
	e.conn.clearActiveTx(e.tx)
	return err
}

func (e *sqliteTxExecutor) Rollback() error {
	err := e.tx.Rollback()
	e.conn.clearActiveTx(e.tx)
	return err
}

func (e *sqliteTxExecutor) Close() error {
	return nil // sql.Tx has no close; Commit/Rollback end it
}

type errScanner struct {
	s interface{ Scan(...any) error }
}

func (s *errScanner) Scan(dest ...any) error {
	err := s.s.Scan(storage.NullSafe(dest)...)
	if err == sql.ErrNoRows {
		return storage.ErrNoRows
	}
	return err
}

// nullRows applies the storage contract's NULL rule to the read-all path.
// *sql.Rows satisfies storage.Rows on its own, which is precisely why this
// wrapper is easy to forget: without it, ReadAll answers differently from
// ReadOne on the same column.
type nullRows struct{ *sql.Rows }

func (r nullRows) Scan(dest ...any) error { return r.Rows.Scan(storage.NullSafe(dest)...) }

var (
	_ storage.Conn            = (*sqliteConn)(nil)
	_ storage.TxExecutor      = (*sqliteConn)(nil)
	_ storage.TxBoundExecutor = (*sqliteTxExecutor)(nil)
	_ ddl.Compiler            = (*sqliteConn)(nil)
)
