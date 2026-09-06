package sqlite

import (
	"webtyp.com/ddl"
	"webtyp.com/storage"
)

type queryer interface {
	Query(string, ...any) (storage.Rows, error)
}

func tableColumns(q queryer, table string) ([]string, error) {
	// PRAGMA table_info does not support parameter binding.
	// Table name comes from ModelName(), which is usually trusted,
	// but we quote it just in case.
	rows, err := q.Query("PRAGMA table_info(\"" + table + "\")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

func (c *sqliteConn) TableColumns(table string) ([]string, error) {
	return tableColumns(c, table)
}

func (e *sqliteTxExecutor) TableColumns(table string) ([]string, error) {
	return tableColumns(e, table)
}

func tables(q queryer) ([]string, error) {
	rows, err := q.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

func columns(q queryer, table string) ([]ddl.ColumnInfo, error) {
	rows, err := q.Query("PRAGMA table_info(\"" + table + "\")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []ddl.ColumnInfo
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, ddl.ColumnInfo{
			Name:    name,
			Type:    ctype,
			NotNull: notnull == 1,
			PK:      pk > 0,
		})
	}
	return cols, rows.Err()
}

// Tables returns all user-defined table names in the database.
func (c *sqliteConn) Tables() ([]string, error) {
	return tables(c)
}

// Columns returns full column metadata for the given table using PRAGMA table_info.
func (c *sqliteConn) Columns(table string) ([]ddl.ColumnInfo, error) {
	return columns(c, table)
}

// Tables returns all user-defined table names in the database.
func (e *sqliteTxExecutor) Tables() ([]string, error) {
	return tables(e)
}

// Columns returns full column metadata for the given table using PRAGMA table_info.
func (e *sqliteTxExecutor) Columns(table string) ([]ddl.ColumnInfo, error) {
	return columns(e, table)
}

// Ensure both executors implement TableIntrospector
var _ ddl.TableIntrospector = (*sqliteConn)(nil)
var _ ddl.TableIntrospector = (*sqliteTxExecutor)(nil)

// Ensure both executors implement ddl.SchemaInspector
var _ ddl.SchemaInspector = (*sqliteConn)(nil)
var _ ddl.SchemaInspector = (*sqliteTxExecutor)(nil)
