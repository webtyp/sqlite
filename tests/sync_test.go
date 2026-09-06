package sqlite_test

import (
	"testing"

	"webtyp.com/ddl"
	"webtyp.com/model"
	"webtyp.com/sqlite"
	"webtyp.com/storage"
)

type SyncUser struct {
	ID   int
	Name string
	Age  int
}

func (u *SyncUser) ModelName() string { return "users" }
func (u *SyncUser) Schema() []model.Field {
	return []model.Field{
		{Name: "id", Type: model.Int(), DB: &model.FieldDB{PK: true, AutoInc: true}},
		{Name: "name", Type: model.Text()},
		{Name: "age", Type: model.Int()},
	}
}
func (u *SyncUser) Pointers() []any { return []any{&u.ID, &u.Name, &u.Age} }

// IsNil, EncodeFields, DecodeFields satisfy model.Model. This fixture only exercises the
// sqlite driver; it never travels over the wire.
func (u *SyncUser) IsNil() bool                      { return u == nil }
func (u *SyncUser) EncodeFields(w model.FieldWriter) {}
func (u *SyncUser) DecodeFields(r model.FieldReader) {}

func TestRegistration(t *testing.T) {
	conn, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open via sqlite.Open: %v", err)
	}
	conn.Close()
}

func TestErrNoRowsMapping(t *testing.T) {
	conn, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer conn.Close()

	dc, ok := sqlite.DDLCompiler(conn)
	if !ok {
		t.Fatalf("no ddl compiler")
	}
	ddldb := ddl.New(conn, dc)
	ddldb.CreateTable(&SyncUser{})

	u := &SyncUser{}
	err = dbReadOne(conn, conn, u, storage.Eq("id", 999))
	if err != storage.ErrNoRows {
		t.Errorf("expected storage.ErrNoRows, got %v", err)
	}
}

func TestTableColumnsIntrospection(t *testing.T) {
	conn, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer conn.Close()

	dc, ok := sqlite.DDLCompiler(conn)
	if !ok {
		t.Fatalf("no ddl compiler")
	}
	ddldb := ddl.New(conn, dc)
	ddldb.CreateTable(&SyncUser{})

	exec, ok := conn.(ddl.TableIntrospector)
	if !ok {
		t.Fatalf("not TableIntrospector")
	}
	cols, err := exec.TableColumns("users")
	if err != nil {
		t.Fatalf("TableColumns failed: %v", err)
	}

	expected := map[string]bool{"id": true, "name": true, "age": true}
	if len(cols) != len(expected) {
		t.Errorf("expected %d columns, got %d", len(expected), len(cols))
	}
	for _, c := range cols {
		if !expected[c] {
			t.Errorf("unexpected column: %s", c)
		}
	}
}

type SyncNewUser struct {
	ID   int
	Name string
	Age  int
	Bio  string
}

func (u *SyncNewUser) ModelName() string { return "users" }
func (u *SyncNewUser) Schema() []model.Field {
	return []model.Field{
		{Name: "id", Type: model.Int(), DB: &model.FieldDB{PK: true, AutoInc: true}},
		{Name: "name", Type: model.Text()},
		{Name: "age", Type: model.Int()},
		{Name: "bio", Type: model.Text()},
	}
}
func (u *SyncNewUser) Pointers() []any { return []any{&u.ID, &u.Name, &u.Age, &u.Bio} }

// IsNil, EncodeFields, DecodeFields satisfy model.Model. This fixture only exercises the
// sqlite driver; it never travels over the wire.
func (u *SyncNewUser) IsNil() bool                      { return u == nil }
func (u *SyncNewUser) EncodeFields(w model.FieldWriter) {}
func (u *SyncNewUser) DecodeFields(r model.FieldReader) {}

func TestSync(t *testing.T) {
	conn, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer conn.Close()

	dc, ok := sqlite.DDLCompiler(conn)
	if !ok {
		t.Fatalf("no ddl compiler")
	}
	ddldb := ddl.New(conn, dc)
	ddldb.CreateTable(&SyncUser{})

	// Sync to SyncNewUser (adds 'bio')
	err = ddldb.Sync(&SyncNewUser{})
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	exec, ok := conn.(ddl.TableIntrospector)
	if !ok {
		t.Fatalf("not TableIntrospector")
	}
	cols, _ := exec.TableColumns("users")
	foundBio := false
	for _, c := range cols {
		if c == "bio" {
			foundBio = true
			break
		}
	}
	if !foundBio {
		t.Errorf("expected 'bio' column after Sync, got %v", cols)
	}
}
