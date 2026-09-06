package sqlite_test

import (
	"testing"

	"webtyp.com/ddl"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/sqlite"
	"webtyp.com/storage"
	"webtyp.com/storage/conformance"
)

type User struct {
	ID   int
	Name string
	Age  int
}

type Order struct {
	ID     int
	UserID int
	Amount float64
}

func (o *Order) ModelName() string {
	return "orders"
}

func (o *Order) Schema() []model.Field {
	return []model.Field{
		{Name: "id", Type: model.Int(), DB: &model.FieldDB{PK: true, AutoInc: true}},
		{Name: "user_id", Type: model.Int()},
		{Name: "amount", Type: model.Float()},
	}
}

func (o *Order) SchemaExt() []model.FieldExt {
	return []model.FieldExt{
		{Field: model.Field{Name: "user_id", Type: model.Int()}, Ref: "users", RefColumn: "id"},
	}
}

func (o *Order) Pointers() []any {
	return []any{&o.ID, &o.UserID, &o.Amount}
}

// IsNil, EncodeFields, DecodeFields satisfy model.Model.
func (o *Order) IsNil() bool                      { return o == nil }
func (o *Order) EncodeFields(w model.FieldWriter) {}
func (o *Order) DecodeFields(r model.FieldReader) {}

func TestComplexQueriesAndJoins(t *testing.T) {
	conn, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer conn.Close()

	err = conn.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT,
			age INTEGER
		);
		CREATE TABLE orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER,
			amount REAL
		);
	`)
	if err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}

	// Insert test data
	users := []*User{
		{Name: "Alice", Age: 30},
		{Name: "Bob", Age: 25},
		{Name: "Charlie", Age: 30},
	}
	for _, u := range users {
		if err := dbCreate(conn, conn, u); err != nil {
			t.Fatalf("Create user failed: %v", err)
		}
	}

	orders := []*Order{
		{UserID: 1, Amount: 100.5},
		{UserID: 1, Amount: 200.0},
		{UserID: 2, Amount: 50.0},
	}
	for _, o := range orders {
		if err := dbCreate(conn, conn, o); err != nil {
			t.Fatalf("Create order failed: %v", err)
		}
	}

	// Test GroupBy, OrderBy, Limit, Offset
	var results []*User
	err = dbReadAll(conn, conn, &User{}, func() model.Model { return &User{} },
		readAllOpts{OrderBy: []storage.Order{storage.Desc("age")}, GroupBy: []string{"age"}, Limit: 1, Offset: 0},
		func(m model.Model) { results = append(results, m.(*User)) },
	)
	if err != nil {
		t.Fatalf("ReadAll with GroupBy/OrderBy/Limit/Offset failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Age != 30 {
		t.Errorf("expected age 30, got %d", results[0].Age)
	}

	// Test JOIN query via raw SQL
	err = conn.Exec(`
		CREATE TABLE user_totals AS
		SELECT u.name, SUM(o.amount) as total
		FROM users u
		JOIN orders o ON u.id = o.user_id
		GROUP BY u.name;
	`)
	if err != nil {
		t.Fatalf("JOIN query failed: %v", err)
	}

	var totals []UserTotalModel
	err = dbReadAll(conn, conn, &UserTotalModel{}, func() model.Model { return &UserTotalModel{} },
		readAllOpts{OrderBy: []storage.Order{storage.Desc("total")}},
		func(m model.Model) { totals = append(totals, *m.(*UserTotalModel)) },
	)
	if err != nil {
		t.Fatalf("ReadAll for user_totals failed: %v", err)
	}

	if len(totals) != 2 {
		t.Fatalf("expected 2 totals, got %d", len(totals))
	}
	if totals[0].Name != "Alice" || totals[0].Total != 300.5 {
		t.Errorf("expected Alice with 300.5, got %s with %v", totals[0].Name, totals[0].Total)
	}
	if totals[1].Name != "Bob" || totals[1].Total != 50.0 {
		t.Errorf("expected Bob with 50.0, got %s with %v", totals[1].Name, totals[1].Total)
	}
}

// UserTotalModel is a model for testing the temp table
type UserTotalModel struct {
	Name  string
	Total float64
}

func (u *UserTotalModel) ModelName() string { return "user_totals" }
func (u *UserTotalModel) Schema() []model.Field {
	return []model.Field{
		{Name: "name", Type: model.Text()},
		{Name: "total", Type: model.Float()},
	}
}
func (u *UserTotalModel) Pointers() []any { return []any{&u.Name, &u.Total} }

// IsNil, EncodeFields, DecodeFields satisfy model.Model.
func (u *UserTotalModel) IsNil() bool                      { return u == nil }
func (u *UserTotalModel) EncodeFields(w model.FieldWriter) {}
func (u *UserTotalModel) DecodeFields(r model.FieldReader) {}

func (u *User) ModelName() string {
	return "users"
}

func (u *User) Schema() []model.Field {
	return []model.Field{
		{Name: "id", Type: model.Int(), DB: &model.FieldDB{PK: true, AutoInc: true}},
		{Name: "name", Type: model.Text()},
		{Name: "age", Type: model.Int()},
	}
}

func (u *User) Pointers() []any {
	return []any{&u.ID, &u.Name, &u.Age}
}

// IsNil, EncodeFields, DecodeFields satisfy model.Model.
func (u *User) IsNil() bool                      { return u == nil }
func (u *User) EncodeFields(w model.FieldWriter) {}
func (u *User) DecodeFields(r model.FieldReader) {}

func TestSqliteAdapter(t *testing.T) {
	// Setup
	conn, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer conn.Close()

	// Create table
	err = conn.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT,
			age INTEGER
		);
	`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	// Test Create
	user := &User{Name: "Alice", Age: 30}
	if err := dbCreate(conn, conn, user); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Test ReadOne
	readUser := &User{}
	if err := dbReadOne(conn, conn, readUser, storage.Eq("name", "Alice")); err != nil {
		t.Fatalf("ReadOne failed: %v", err)
	}
	if readUser.Name != "Alice" {
		t.Errorf("expected name Alice, got %s", readUser.Name)
	}

	// Test Update
	if err := dbUpdate(conn, conn, &User{ID: readUser.ID, Name: "Alice", Age: 31}, storage.Eq("name", "Alice")); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify Update
	readUser = &User{}
	if err := dbReadOne(conn, conn, readUser, storage.Eq("name", "Alice")); err != nil {
		t.Fatalf("ReadOne after Update failed: %v", err)
	}
	if readUser.Age != 31 {
		t.Errorf("expected age 31, got %d", readUser.Age)
	}

	// Test ReadAll
	dbCreate(conn, conn, &User{Name: "Bob", Age: 25})
	var users []*User
	err = dbReadAll(conn, conn, &User{}, func() model.Model { return &User{} }, readAllOpts{},
		func(m model.Model) { users = append(users, m.(*User)) })
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}

	// Test IN operator
	var inUsers []*User
	err = dbReadAll(conn, conn, &User{}, func() model.Model { return &User{} },
		readAllOpts{Conditions: []storage.Condition{storage.In("name", []any{"Alice", "Bob"})}},
		func(m model.Model) { inUsers = append(inUsers, m.(*User)) })
	if err != nil {
		t.Fatalf("IN ReadAll failed: %v", err)
	}
	if len(inUsers) != 2 {
		t.Errorf("expected 2 users from IN, got %d", len(inUsers))
	}

	// Test Delete
	if err := dbDelete(conn, conn, &User{}, storage.Eq("name", "Bob")); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify Delete
	users = nil
	err = dbReadAll(conn, conn, &User{}, func() model.Model { return &User{} }, readAllOpts{},
		func(m model.Model) { users = append(users, m.(*User)) })
	if err != nil {
		t.Fatalf("ReadAll after Delete failed: %v", err)
	}
	if len(users) != 1 {
		t.Errorf("expected 1 user, got %d", len(users))
	}
}

type errorExecutor struct {
	storage.Conn
}

func (e *errorExecutor) Close() error {
	return fmt.Err("close error")
}

func (e *errorExecutor) Exec(query string, args ...any) error {
	return fmt.Err("exec error")
}

func TestCloseError(t *testing.T) {
	fakeConn := &errorExecutor{}
	err := fakeConn.Close()
	if err == nil {
		t.Fatalf("expected error when closing db, got nil")
	}
}

func TestExecSQLError(t *testing.T) {
	fakeConn := &errorExecutor{}
	err := fakeConn.Exec("SELECT 1")
	if err == nil {
		t.Fatalf("expected error when execSQL fails, got nil")
	}
}

type BadModel struct {
	Name string
}

func (b *BadModel) ModelName() string     { return "" }
func (b *BadModel) Schema() []model.Field { return nil }
func (b *BadModel) Pointers() []any       { return nil }

type NoColsModel struct {
	Name string
}

func (n *NoColsModel) ModelName() string     { return "no_cols" }
func (n *NoColsModel) Schema() []model.Field { return nil }
func (n *NoColsModel) Pointers() []any       { return nil }

func TestCreateTable(t *testing.T) {
	conn, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer conn.Close()

	dc, ok := sqlite.DDLCompiler(conn)
	if !ok {
		t.Fatalf("no ddl compiler")
	}
	ddldb := ddl.New(conn, dc)

	err = ddldb.CreateTable(&User{})
	if err != nil {
		t.Fatalf("CreateTable User failed: %v", err)
	}

	// Verify table exists by inserting into it
	user := &User{Name: "Alice", Age: 30}
	if err := dbCreate(conn, conn, user); err != nil {
		t.Fatalf("Create User record failed after CreateTable: %v", err)
	}
}

func TestDropTable(t *testing.T) {
	conn, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer conn.Close()

	dc, ok := sqlite.DDLCompiler(conn)
	if !ok {
		t.Fatalf("no ddl compiler")
	}
	ddldb := ddl.New(conn, dc)

	err = ddldb.CreateTable(&User{})
	if err != nil {
		t.Fatalf("CreateTable User failed: %v", err)
	}

	err = ddldb.DropTable(&User{})
	if err != nil {
		t.Fatalf("DropTable User failed: %v", err)
	}

	// Verify table is gone by attempting to insert
	user := &User{Name: "Alice", Age: 30}
	err = dbCreate(conn, conn, user)
	if err == nil {
		t.Fatalf("Expected error inserting into dropped table, got nil")
	}
}

func TestCreateTableWithFK(t *testing.T) {
	conn, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer conn.Close()

	dc, ok := sqlite.DDLCompiler(conn)
	if !ok {
		t.Fatalf("no ddl compiler")
	}
	ddldb := ddl.New(conn, dc)

	// First create referenced table
	err = ddldb.CreateTable(&User{})
	if err != nil {
		t.Fatalf("CreateTable User failed: %v", err)
	}

	// Then create table with FK
	err = ddldb.CreateTable(&Order{})
	if err != nil {
		t.Fatalf("CreateTable Order (with FK) failed: %v", err)
	}
}

func TestExecutorErrors(t *testing.T) {
	conn, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer conn.Close()

	// Test Exec error
	err = conn.Exec("INVALID SQL")
	if err == nil {
		t.Fatalf("expected error on invalid sql format")
	}

	// Test QueryRow error
	row := conn.QueryRow("SELECT * FROM non_existent")
	err = row.Scan()
	if err == nil {
		t.Fatalf("expected error scanning from invalid table")
	}

	// Test Query error
	_, err = conn.Query("SELECT * FROM non_existent")
	if err == nil {
		t.Fatalf("expected error querying invalid table")
	}

	// BeginTx failure
	sqlDB, ok := sqlite.GetSqlDB(conn)
	if !ok {
		t.Fatalf("could not get sql DB")
	}
	sqlDB.Close() // Force BeginTx to fail

	txExec, ok := conn.(storage.TxExecutor)
	if !ok {
		t.Fatalf("executor does not implement TxExecutor")
	}
	_, err = txExec.BeginTx()
	if err == nil {
		t.Fatalf("expected BeginTx error on closed DB")
	}
}

func TestTxExecutorErrors(t *testing.T) {
	conn, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer conn.Close()

	txExec, ok := conn.(storage.TxExecutor)
	if !ok {
		t.Fatalf("not TxExecutor")
	}
	tx, err := txExec.BeginTx()
	if err != nil {
		t.Fatalf("failed to open tx: %v", err)
	}

	// Test Exec
	err = tx.Exec("INVALID SQL")
	if err == nil {
		t.Fatalf("expected error on invalid tx sql")
	}

	// Test QueryRow
	row := tx.QueryRow("SELECT * FROM non_existent")
	err = row.Scan()
	if err == nil {
		t.Fatalf("expected error scanning tx invalid table")
	}

	// Test Query
	_, err = tx.Query("SELECT * FROM non_existent")
	if err == nil {
		t.Fatalf("expected error querying tx invalid table")
	}

	// Rollback
	err = tx.Rollback()
	if err != nil {
		t.Fatalf("failed rollback: %v", err)
	}

	// Commit on fresh transaction
	tx2, _ := txExec.BeginTx()
	err = tx2.Commit()
	if err != nil {
		t.Fatalf("commit failed: %v", err)
	}
}

func TestTransaction(t *testing.T) {
	conn, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer conn.Close()

	err = conn.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT,
			age INTEGER
		);
	`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	txExec, ok := conn.(storage.TxExecutor)
	if !ok {
		t.Fatalf("not TxExecutor")
	}

	// Test Commit
	tx, err := txExec.BeginTx()
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}
	if err := dbCreate(conn, tx, &User{Name: "Charlie", Age: 40}); err != nil {
		tx.Rollback()
		t.Fatalf("tx Create failed: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Tx commit failed: %v", err)
	}

	// Verify Commit
	readUser := &User{}
	if err := dbReadOne(conn, conn, readUser, storage.Eq("name", "Charlie")); err != nil {
		t.Fatalf("ReadOne failed: %v", err)
	}

	// Test Rollback
	tx, err = txExec.BeginTx()
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}
	if err := dbCreate(conn, tx, &User{Name: "Dave", Age: 50}); err != nil {
		tx.Rollback()
		t.Fatalf("tx Create failed: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Tx rollback failed: %v", err)
	}

	// Verify Rollback
	readUser = &User{}
	if err := dbReadOne(conn, conn, readUser, storage.Eq("name", "Dave")); err == nil {
		t.Errorf("expected Dave not to be found after rollback, but ReadOne succeeded with name: %s", readUser.Name)
	}
}

func TestUpdate_ExplicitPK_MultiRow(t *testing.T) {
	conn, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	dc, ok := sqlite.DDLCompiler(conn)
	if !ok {
		t.Fatalf("no ddl compiler")
	}
	ddldb := ddl.New(conn, dc)

	if err := ddldb.CreateTable(&User{}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	// Insert three users.
	seeds := []User{
		{Name: "Alice", Age: 30},
		{Name: "Bob", Age: 25},
		{Name: "Charlie", Age: 20},
	}
	for i := range seeds {
		if err := dbCreate(conn, conn, &seeds[i]); err != nil {
			t.Fatalf("Create %s: %v", seeds[i].Name, err)
		}
	}

	// Read them back to obtain DB-assigned IDs.
	var users []*User
	err = dbReadAll(conn, conn, &User{}, func() model.Model { return &User{} }, readAllOpts{},
		func(m model.Model) { users = append(users, m.(*User)) })
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("expected 3 users, got %d", len(users))
	}

	// Update each user's age in a loop using an explicit PK condition, inside a Tx.
	txExec, ok := conn.(storage.TxExecutor)
	if !ok {
		t.Fatalf("not TxExecutor")
	}
	tx, err := txExec.BeginTx()
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	for _, u := range users {
		u.Age += 10
		if err := dbUpdate(conn, tx, u, storage.Eq("id", u.ID)); err != nil {
			tx.Rollback()
			t.Fatalf("tx Update: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Tx multi-row Update commit: %v", err)
	}

	// Verify each row was updated independently.
	wantAges := map[int]int{users[0].ID: 40, users[1].ID: 35, users[2].ID: 30}
	for _, u := range users {
		got := &User{}
		if err := dbReadOne(conn, conn, got, storage.Eq("id", u.ID)); err != nil {
			t.Fatalf("ReadOne user %d: %v", u.ID, err)
		}
		if got.Age != wantAges[u.ID] {
			t.Errorf("user %d: expected age %d, got %d", u.ID, wantAges[u.ID], got.Age)
		}
	}
}

func testSchemaInspector(t *testing.T, inspector ddl.SchemaInspector) {
	// Test Tables()
	tables, err := inspector.Tables()
	if err != nil {
		t.Fatalf("Tables() failed: %v", err)
	}

	// sqlite_master might contain other things, but our query filters sqlite_%
	expectedTables := map[string]bool{"users": true, "orders": true}
	foundCount := 0
	for _, table := range tables {
		if expectedTables[table] {
			foundCount++
		}
	}
	if foundCount != 2 {
		t.Errorf("expected 2 user tables, got %d from %v", foundCount, tables)
	}

	// Test Columns() for 'users'
	cols, err := inspector.Columns("users")
	if err != nil {
		t.Fatalf("Columns('users') failed: %v", err)
	}

	if len(cols) != 3 {
		t.Fatalf("expected 3 columns for 'users', got %d", len(cols))
	}

	colMap := make(map[string]ddl.ColumnInfo)
	for _, col := range cols {
		colMap[col.Name] = col
	}

	idCol, ok := colMap["id"]
	if !ok {
		t.Errorf("column 'id' not found")
	} else {
		if !idCol.PK {
			t.Errorf("expected 'id' to be PK")
		}
		// SQLite type might be INTEGER or INT depending on how it was created
		if idCol.Type != "INTEGER" {
			t.Errorf("expected type INTEGER, got %s", idCol.Type)
		}
	}

	nameCol, ok := colMap["name"]
	if !ok {
		t.Errorf("column 'name' not found")
	} else {
		if !nameCol.NotNull {
			t.Errorf("expected 'name' to be NOT NULL")
		}
		if nameCol.Type != "TEXT" {
			t.Errorf("expected type TEXT, got %s", nameCol.Type)
		}
	}

	ageCol, ok := colMap["age"]
	if !ok {
		t.Errorf("column 'age' not found")
	} else {
		if ageCol.NotNull {
			t.Errorf("expected 'age' to be nullable")
		}
	}
}

func TestSchemaInspector(t *testing.T) {
	conn, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer conn.Close()

	err = conn.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			age INTEGER
		);
		CREATE TABLE orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER,
			amount REAL
		);
	`)
	if err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}

	t.Run("Executor", func(t *testing.T) {
		inspector, ok := conn.(ddl.SchemaInspector)
		if !ok {
			t.Fatalf("executor does not implement ddl.SchemaInspector")
		}
		testSchemaInspector(t, inspector)
	})

	t.Run("TxExecutor", func(t *testing.T) {
		txExec, ok := conn.(storage.TxExecutor)
		if !ok {
			t.Fatalf("not TxExecutor")
		}
		tx, err := txExec.BeginTx()
		if err != nil {
			t.Fatalf("failed to get tx executor: %v", err)
		}
		defer tx.Rollback()

		inspector, ok := tx.(ddl.SchemaInspector)
		if !ok {
			t.Fatalf("tx executor does not implement ddl.SchemaInspector")
		}
		testSchemaInspector(t, inspector)
	})
}

func TestSqliteAdapter_DBConformance(t *testing.T) {
	conformance.Run(t, conformance.Factory{
		Name: "sqlite-adapter",
		New: func(t *testing.T, models ...model.Model) storage.Conn {
			conn, err := sqlite.Open(":memory:")
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			dc, ok := sqlite.DDLCompiler(conn)
			if !ok {
				t.Fatalf("compiler does not support DDL")
			}
			db := ddl.New(conn, dc)
			for _, m := range models {
				if err := db.CreateTable(m); err != nil {
					t.Fatalf("CreateTable: %v", err)
				}
			}
			return conn
		},
	})
}
