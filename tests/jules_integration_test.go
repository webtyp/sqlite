package sqlite_test

import (
	"testing"

	"webtyp.com/ddl"
	"webtyp.com/model"
	"webtyp.com/sqlite"
	"webtyp.com/storage"
)

type SimpleUser struct {
	ID    string
	Email string
}

func (s *SimpleUser) ModelName() string { return "simple_users" }
func (s *SimpleUser) Schema() []model.Field {
	return []model.Field{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "email", Type: model.Text(), DB: &model.FieldDB{Unique: true}},
	}
}
func (s *SimpleUser) Pointers() []any { return []any{&s.ID, &s.Email} }

// IsNil, EncodeFields, DecodeFields satisfy model.Model. This fixture only exercises the
// sqlite driver; it never travels over the wire.
func (s *SimpleUser) IsNil() bool                      { return s == nil }
func (s *SimpleUser) EncodeFields(w model.FieldWriter) {}
func (s *SimpleUser) DecodeFields(r model.FieldReader) {}

type SimpleSession struct {
	ID     string
	UserID string
}

func (s *SimpleSession) ModelName() string { return "simple_sessions" }
func (s *SimpleSession) Schema() []model.Field {
	return []model.Field{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "user_id", Type: model.Text()},
	}
}
func (s *SimpleSession) SchemaExt() []model.FieldExt {
	return []model.FieldExt{
		{Field: model.Field{Name: "user_id", Type: model.Text()}, Ref: "simple_users", RefColumn: "id"},
	}
}
func (s *SimpleSession) Pointers() []any { return []any{&s.ID, &s.UserID} }

// IsNil, EncodeFields, DecodeFields satisfy model.Model. This fixture only exercises the
// sqlite driver; it never travels over the wire.
func (s *SimpleSession) IsNil() bool                      { return s == nil }
func (s *SimpleSession) EncodeFields(w model.FieldWriter) {}
func (s *SimpleSession) DecodeFields(r model.FieldReader) {}

func TestJulesScenario(t *testing.T) {
	conn, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer conn.Close()

	dc, ok := sqlite.DDLCompiler(conn)
	if !ok {
		t.Fatalf("no DDL compiler")
	}
	ddldb := ddl.New(conn, dc)

	// Create SimpleUser table
	err = ddldb.CreateTable(&SimpleUser{})
	if err != nil {
		t.Fatalf("failed to create SimpleUser table: %v", err)
	}

	// Calling CreateTable twice should return success (IF NOT EXISTS)
	err = ddldb.CreateTable(&SimpleUser{})
	if err != nil {
		t.Fatalf("failed to create SimpleUser table (second time): %v", err)
	}

	// Insert into SimpleUser
	user := &SimpleUser{ID: "user_123", Email: "test@example.com"}
	err = dbCreate(conn, conn, user)
	if err != nil {
		t.Fatalf("failed to create simple user record: %v", err)
	}

	// Read from SimpleUser
	var readUser SimpleUser
	err = dbReadOne(conn, conn, &readUser, storage.Eq("id", "user_123"))
	if err != nil {
		t.Fatalf("failed to read simple user record: %v", err)
	}
	if readUser.Email != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got '%s'", readUser.Email)
	}

	// Create SimpleSession table
	err = ddldb.CreateTable(&SimpleSession{})
	if err != nil {
		t.Fatalf("failed to create SimpleSession table: %v", err)
	}

	// Insert into SimpleSession
	session := &SimpleSession{ID: "sess_abc", UserID: "user_123"}
	err = dbCreate(conn, conn, session)
	if err != nil {
		t.Fatalf("failed to create simple session record: %v", err)
	}
}
