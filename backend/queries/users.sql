-- name: GetUserByID :one
SELECT id, email, role, status, organization_id, created_at
FROM users
WHERE id = $1;