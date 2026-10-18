-- name: CreateUser :one
INSERT INTO users (display_name, email, password_hash)
VALUES (@display_name, @email, @password_hash)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = @id;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower(@email::text);
