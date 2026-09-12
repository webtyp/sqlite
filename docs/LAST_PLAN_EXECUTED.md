---
PLAN: "fix: a NULL column scans as the Go zero value"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
>
> **Phase B** of
> [`NULLABLE_COLUMNS_MASTER_PLAN.md`](https://github.com/webtyp/docs/blob/main/NULLABLE_COLUMNS_MASTER_PLAN.md).
>
> **Depends on phase A** (`webtyp.com/storage` publishing `storage.NullSafe`):
> as the first line of work, `go get webtyp.com/storage@latest`. Never add a
> `replace`, never invent a version.

# Plan — `webtyp.com/sqlite`: stop letting `database/sql` decide what NULL means

## 0. Context (verified against the repo — do not re-diagnose)

This backend hands the model's raw pointers (`*string`, `*int64`, …) straight to
`database/sql`, which refuses a NULL column:

```
sql: Scan error on column index 1, name "email": converting NULL to string is unsupported
```

Meanwhile `storage/mem` — the backend consumers unit-test against — returns the
Go zero value for the same row. So a consumer's suite passes and production
fails on the same code. Phase A settled the rule once, in the package that owns
the contract:

```
A NULL column scans as the Go ZERO VALUE, in every backend.
```

Phase A also shipped the mechanism: `storage.NullSafe(dest)` wraps scan
destinations so `database/sql` hands the raw value to `storage.ScanAny` instead
of converting it itself. This phase applies it at this backend's two scan
seams. **No conversion logic is written here** — if a value converts wrongly,
the fix belongs in `storage.ScanAny`, not in this repo.

There are exactly two seams, both in `executor.go`:

- `QueryRow` → returns `&errScanner{...}`, whose `Scan` forwards to `database/sql`.
- `Query` → returns `tx.Query(...)` / `c.db.Query(...)` **directly**: a
  `*sql.Rows` satisfies `storage.Rows` structurally, so today there is no
  wrapper at all on the read-all path.

This plan adds no public API. It changes behaviour only: a NULL that used to
error now yields the zero value.

## Quality rules

```
RULE: no conversion logic in this repo — every value goes through storage.ScanAny.
RULE: every repeated string is a named constant; string literals forbidden in logic.
RULE: do not change the ErrNoRows translation that errScanner already performs.
```

## Stage 1 — the single-row seam

**File:** `executor.go`.

`errScanner.Scan` currently forwards the destinations untouched:

```go
func (s *errScanner) Scan(dest ...any) error {
	err := s.s.Scan(dest...)
	if err == sql.ErrNoRows {
		return storage.ErrNoRows
	}
	return err
}
```

Wrap the destinations, keeping the `ErrNoRows` translation exactly as it is:

```go
func (s *errScanner) Scan(dest ...any) error {
	err := s.s.Scan(storage.NullSafe(dest)...)
	if err == sql.ErrNoRows {
		return storage.ErrNoRows
	}
	return err
}
```

## Stage 2 — the read-all seam

**File:** `executor.go`.

`Query` returns the `*sql.Rows` directly, so nothing applies the rule on this
path. Add a wrapper next to `errScanner`:

```go
// nullRows applies the storage contract's NULL rule to the read-all path.
// *sql.Rows satisfies storage.Rows on its own, which is precisely why this
// wrapper is easy to forget: without it, ReadAll answers differently from
// ReadOne on the same column.
type nullRows struct{ *sql.Rows }

func (r nullRows) Scan(dest ...any) error { return r.Rows.Scan(storage.NullSafe(dest)...) }
```

Embedding `*sql.Rows` keeps `Next`, `Close` and `Err` forwarding untouched;
only `Scan` is overridden.

Then return it from **both** branches of `Query`:

```go
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
```

Apply the identical change to `sqliteTxExecutor.Query` (same file, around
line 112) — it has the same shape and the same defect.

**Do not** wrap the rows used by `introspect.go`: those scan into local
`*string`/`*int` variables for `PRAGMA` output, not into a model, and they are
not part of the storage contract.

## Stage 3 — conformance proves it

**File:** the existing conformance test file in this repo (the one that calls
`conformance.Run`/the exported suite from `webtyp.com/storage/conformance`).

Phase A added a nullable column and a `NullScansAsZero` case to the shared
suite. This repo already runs that suite against a real SQLite connection, so
the only required change is the dependency bump — **verify the case actually
runs and passes** rather than assuming it.

If this repo pins the conformance suite through a version that predates phase A,
`go get webtyp.com/storage@latest` is what picks it up.

## Acceptance criteria

1. `go build ./...`, `go vet ./...`, `go test ./...` green.
2. The `NullScansAsZero` conformance case runs against real SQLite and passes.
3. `grep -rn "\.Scan(dest\.\.\.)" --include='*.go' .` → no hit outside
   `introspect.go`: every model-facing scan goes through `storage.NullSafe`.
4. `grep -rn "converting NULL" --include='*.go' .` → empty.
5. `go.mod` requires the phase A tag of `webtyp.com/storage`; no `replace`.

## Out of scope

- Conversion rules (`[]byte` → `string`, numeric text) — they live in
  `storage.ScanAny`, phase A.
- `webtyp/postgres`, which carries the identical defect — phase C.

| Stage | Files | Action |
|---|---|---|
| 1 | `executor.go` | `errScanner.Scan` wraps dest in `storage.NullSafe` |
| 2 | `executor.go` | `nullRows` wrapper returned by both `Query` implementations |
| 3 | conformance test file | verify `NullScansAsZero` runs and passes on real SQLite |
