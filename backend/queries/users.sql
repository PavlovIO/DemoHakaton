-- name: GetUserByID :one
SELECT id, email, password_hash, role, status, organization_id,
       blocked_reason, last_login_at, created_at, updated_at
FROM users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, role, status, organization_id,
       blocked_reason, last_login_at, created_at, updated_at
FROM users
WHERE email = $1;

-- name: CreateUser :one
INSERT INTO users (email, password_hash, role, organization_id)
VALUES ($1, $2, $3, $4)
RETURNING id, email, role, status, organization_id, created_at;

-- name: UpdateLastLogin :exec
UPDATE users SET last_login_at = now() WHERE id = $1;

-- name: BlockUser :one
UPDATE users SET status = 'blocked', blocked_reason = $2, updated_at = now()
WHERE id = $1
RETURNING id, email, role, status, organization_id, blocked_reason, created_at;

-- name: UnblockUser :one
UPDATE users SET status = 'active', blocked_reason = NULL, updated_at = now()
WHERE id = $1
RETURNING id, email, role, status, organization_id, blocked_reason, created_at;

-- name: ListUsers :many
SELECT id, email, role, status, organization_id, created_at
FROM users
WHERE (sqlc.narg('role')::user_role IS NULL OR role = sqlc.narg('role')::user_role)
  AND (sqlc.narg('status')::user_status IS NULL OR status = sqlc.narg('status')::user_status)
  AND (sqlc.narg('q')::text IS NULL OR email ILIKE '%' || sqlc.narg('q')::text || '%' ESCAPE '\')
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountUsers :one
SELECT count(*) FROM users
WHERE (sqlc.narg('role')::user_role IS NULL OR role = sqlc.narg('role')::user_role)
  AND (sqlc.narg('status')::user_status IS NULL OR status = sqlc.narg('status')::user_status)
  AND (sqlc.narg('q')::text IS NULL OR email ILIKE '%' || sqlc.narg('q')::text || '%' ESCAPE '\');

-- name: ListUsersByOrganization :many
-- Пользователи компании для карточки админа (BE-12): колонки ListUsers, без password_hash.
SELECT id, email, role, status, organization_id, created_at
FROM users
WHERE organization_id = $1
ORDER BY created_at, id;

-- name: GetUserForUpdate :one
-- Блокировка пользователя для block/unblock (BE-12): два параллельных решения не запишут
-- историю дважды — второе увидит уже новый статус и ответит без записей.
SELECT id, email, password_hash, role, status, organization_id,
       blocked_reason, last_login_at, created_at, updated_at
FROM users
WHERE id = $1
FOR UPDATE;
