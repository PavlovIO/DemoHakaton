-- name: CreateSession :one
INSERT INTO sessions (token_hash, user_id, expires_at, ip, user_agent)
VALUES ($1, $2, $3, $4, $5)
RETURNING token_hash, user_id, expires_at, created_at;

-- name: GetSessionByTokenHash :one
SELECT s.token_hash, s.user_id, s.expires_at, s.created_at, s.last_seen_at,
       s.ip, s.user_agent,
       u.email, u.role, u.status, u.organization_id
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1
  AND s.expires_at > now()
  AND u.status = 'active';

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = now() WHERE token_hash = $1;

-- name: DeleteSessionByTokenHash :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteSessionsByUserID :exec
DELETE FROM sessions WHERE user_id = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at < now();