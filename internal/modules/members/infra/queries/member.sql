-- name: SaveMember :exec
INSERT INTO members.member (id, email, name) VALUES ($1, $2, $3);

-- name: FindMemberByID :one
SELECT id, email, name, created_at FROM members.member WHERE id = $1;
