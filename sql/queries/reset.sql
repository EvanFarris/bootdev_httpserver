-- name: ResetUsers :exec
DELETE FROM users;

-- name: ResetChirps :exec
DELETE FROM chirps;

-- name: ResetRefreshTokens :exec
DELETE FROM refresh_tokens;