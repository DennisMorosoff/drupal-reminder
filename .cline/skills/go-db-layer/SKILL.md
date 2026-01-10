---
name: go-db-layer
description: Implement database operations, repositories, or SQL queries in Go. Use when the user asks to "save data", "create a model", or "fix a query".
---

# Go Database Development Standards

Follow these rules when working with the database layer:

## 1. Context is King
- Every repository method MUST accept `ctx context.Context` as the first argument.
- Pass this context to the query execution: `db.QueryRowContext(ctx, ...)` or `gorm.WithContext(ctx)`.

## 2. Models & Structs
- Define data models in the `internal/domain` or `internal/models` package.
- Use struct tags for DB mapping: `db:"user_id" json:"user_id"`.

## 3. Handling Nulls and Errors
- Handle `sql.ErrNoRows` explicitly. Return a domain-specific error like `ErrUserNotFound`.
- Use `sql.NullString` or pointers `*string` only if the column is truly nullable in the DB schema.

## 4. Concurrency Safety
- Remember that database handlers might be called from multiple goroutines.
- Do not store request-scoped state in the Repository struct.

## Example Pattern
```go
func (r *UserRepository) GetByID(ctx context.Context, id int64) (*User, error) {
    query := `SELECT id, name FROM users WHERE id = $1`
    var u User
    err := r.db.GetContext(ctx, &u, query, id)
    if err != nil {
         if errors.Is(err, sql.ErrNoRows) {
             return nil, domain.ErrNotFound
         }
         return nil, fmt.Errorf("repo.GetByID: %w", err)
    }
    return &u, nil
}
```
