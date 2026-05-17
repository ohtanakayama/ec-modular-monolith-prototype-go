// Package member is the aggregate root package for the members BC.
// Fields are package-private; consumers go through factory + getters.
// Design rationale: ADR 0002 (domain building blocks).
package member

import "time"

// Member is the aggregate root.
type Member struct {
	id        string
	email     Email
	name      MemberName
	createdAt time.Time
}

// New constructs a Member from already-validated VOs and identity.
// Use this both for fresh registration (id = freshly minted, createdAt = now)
// and for hydrating from a repository row.
func New(id string, email Email, name MemberName, createdAt time.Time) *Member {
	return &Member{id: id, email: email, name: name, createdAt: createdAt}
}

func (m *Member) ID() string           { return m.id }
func (m *Member) Email() Email         { return m.email }
func (m *Member) Name() MemberName     { return m.name }
func (m *Member) CreatedAt() time.Time { return m.createdAt }
