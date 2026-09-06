package sqlite_test

import (
	"webtyp.com/model"
	"webtyp.com/storage"
)

// execer is satisfied by both storage.Conn and storage.TxBoundExecutor — the shared surface
// these test helpers need to actually run a compiled statement. Compile always comes from the
// storage.Conn (only it holds the compiler); exec is whichever executor the operation should
// run through (the conn itself, or an active Tx).
type execer interface {
	Exec(query string, args ...any) error
	QueryRow(query string, args ...any) storage.Scanner
	Query(query string, args ...any) (storage.Rows, error)
}

// These helpers mirror what webtyp/orm builds internally (see storage/conformance's own
// create/readOne/readAll/update/delete) — this package tests the raw storage.Conn contract
// directly, without depending on the ergonomic orm layer.

func dbCreate(conn storage.Conn, exec execer, m model.Model) error {
	schema := m.Schema()
	ptrs := m.Pointers()

	// Skip PK+AutoInc columns — the DB assigns them, same convention as orm.DB.Create
	// (see model.Field.EncodeFields' ActionCreate branch).
	var columns []string
	var insertSchema []model.Field
	var insertPtrs []any
	for i, f := range schema {
		if f.IsPK() && f.IsAutoInc() {
			continue
		}
		columns = append(columns, f.Name)
		insertSchema = append(insertSchema, f)
		insertPtrs = append(insertPtrs, ptrs[i])
	}

	q := storage.Query{
		Action: storage.ActionCreate, Table: m.ModelName(),
		Columns: columns, Values: model.ReadValues(insertSchema, insertPtrs),
	}
	plan, err := conn.Compile(q, m)
	if err != nil {
		return err
	}
	return exec.Exec(plan.Query, plan.Args...)
}

func dbReadOne(conn storage.Conn, exec execer, m model.Model, conds ...storage.Condition) error {
	q := storage.Query{Action: storage.ActionReadOne, Table: m.ModelName(), Conditions: conds, Limit: 1}
	plan, err := conn.Compile(q, m)
	if err != nil {
		return err
	}
	return exec.QueryRow(plan.Query, plan.Args...).Scan(m.Pointers()...)
}

type readAllOpts struct {
	Conditions []storage.Condition
	OrderBy    []storage.Order
	GroupBy    []string
	Limit      int
	Offset     int
}

func dbReadAll(conn storage.Conn, exec execer, template model.Model, newModel func() model.Model, opts readAllOpts, each func(model.Model)) error {
	q := storage.Query{
		Action: storage.ActionReadAll, Table: template.ModelName(),
		Conditions: opts.Conditions, OrderBy: opts.OrderBy, GroupBy: opts.GroupBy,
		Limit: opts.Limit, Offset: opts.Offset,
	}
	plan, err := conn.Compile(q, template)
	if err != nil {
		return err
	}
	rows, err := exec.Query(plan.Query, plan.Args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		nm := newModel()
		if err := rows.Scan(nm.Pointers()...); err != nil {
			return err
		}
		each(nm)
	}
	return rows.Err()
}

func dbUpdate(conn storage.Conn, exec execer, m model.Model, conds ...storage.Condition) error {
	schema := m.Schema()
	columns := make([]string, len(schema))
	for i, f := range schema {
		columns[i] = f.Name
	}
	q := storage.Query{
		Action: storage.ActionUpdate, Table: m.ModelName(),
		Columns: columns, Values: model.ReadValues(schema, m.Pointers()), Conditions: conds,
	}
	plan, err := conn.Compile(q, m)
	if err != nil {
		return err
	}
	return exec.Exec(plan.Query, plan.Args...)
}

func dbDelete(conn storage.Conn, exec execer, m model.Model, conds ...storage.Condition) error {
	q := storage.Query{Action: storage.ActionDelete, Table: m.ModelName(), Conditions: conds}
	plan, err := conn.Compile(q, m)
	if err != nil {
		return err
	}
	return exec.Exec(plan.Query, plan.Args...)
}
