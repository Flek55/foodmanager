// Package household manages households, their members and invite codes.
//
// Access rule used everywhere: if the caller isn't a member, the household
// "doesn't exist" (404, not 403), so ids of other families can't be probed.
package household

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"foodmanager/internal/apperr"
	"foodmanager/internal/events"
	"foodmanager/internal/shopping"
	"foodmanager/internal/store"
)

const (
	RoleOwner  = "owner"
	RoleMember = "member"

	maxNameLen         = 60
	DefaultInviteUses  = 10
	MaxInviteUses      = 50
	DefaultInviteTTL   = 7 * 24 * time.Hour
	MaxInviteTTL       = 30 * 24 * time.Hour
	maxActiveInvites   = 20
	maxHouseholdMember = 30
)

var (
	errHouseholdNotFound = apperr.NotFoundCode(apperr.CodeHouseholdNotFound, "семья не найдена")
	errInviteNotFound    = apperr.NotFound("код не найден или больше не действует")
)

type Service struct {
	pool *pgxpool.Pool
	q    *store.Queries
	pub  events.Publisher
}

func NewService(pool *pgxpool.Pool, pub events.Publisher) *Service {
	return &Service{pool: pool, q: store.New(pool), pub: pub}
}

// notify tells open clients that the household's name or members changed.
func (s *Service) notify(householdID uuid.UUID) {
	s.pub.Publish(householdID, events.Event{Kind: events.KindHousehold})
}

// Detail is a household as seen by one of its members.
type Detail struct {
	ID      uuid.UUID
	Name    string
	Role    string // role of the viewing user
	Members []store.ListMembersRow
}

type Invite struct {
	Code          string // canonical, without dash
	MaxUses       int
	Uses          int
	ExpiresAt     time.Time
	CreatedByName string
}

func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID) ([]store.ListUserHouseholdsRow, error) {
	return s.q.ListUserHouseholds(ctx, userID)
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, name string) (Detail, error) {
	name, err := normalizeName(name)
	if err != nil {
		return Detail{}, err
	}
	var id uuid.UUID
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		h, err := q.CreateHousehold(ctx, store.CreateHouseholdParams{Name: name, CreatedBy: &userID})
		if err != nil {
			return err
		}
		id = h.ID
		if _, err = q.AddMember(ctx, store.AddMemberParams{HouseholdID: h.ID, UserID: userID, Role: RoleOwner}); err != nil {
			return err
		}
		version, err := q.BumpHouseholdVersion(ctx, h.ID)
		if err != nil {
			return err
		}
		return shopping.SeedCategories(ctx, q, h.ID, version)
	})
	if err != nil {
		return Detail{}, err
	}
	return s.Get(ctx, userID, id)
}

func (s *Service) Get(ctx context.Context, userID, householdID uuid.UUID) (Detail, error) {
	return getDetail(ctx, s.q, userID, householdID)
}

func (s *Service) Rename(ctx context.Context, userID, householdID uuid.UUID, name string) (Detail, error) {
	name, err := normalizeName(name)
	if err != nil {
		return Detail{}, err
	}
	role, err := s.role(ctx, s.q, userID, householdID)
	if err != nil {
		return Detail{}, err
	}
	if role != RoleOwner {
		return Detail{}, apperr.Forbidden("переименовать семью может только владелец")
	}
	if err := s.q.RenameHousehold(ctx, store.RenameHouseholdParams{ID: householdID, Name: name}); err != nil {
		return Detail{}, err
	}
	s.notify(householdID)
	return s.Get(ctx, userID, householdID)
}

// RemoveMember removes targetID from the household. An owner can remove
// anyone; any member can remove themselves (leave). When the last owner
// leaves, ownership passes to the longest-standing member; when the last
// member leaves, the household is deleted.
func (s *Service) RemoveMember(ctx context.Context, actorID, householdID, targetID uuid.UUID) error {
	err := s.removeMember(ctx, actorID, householdID, targetID)
	if err == nil {
		s.notify(householdID)
	}
	return err
}

func (s *Service) removeMember(ctx context.Context, actorID, householdID, targetID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := q.LockHousehold(ctx, householdID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errHouseholdNotFound
			}
			return err
		}
		actorRole, err := s.role(ctx, q, actorID, householdID)
		if err != nil {
			return err
		}
		if actorID != targetID && actorRole != RoleOwner {
			return apperr.Forbidden("исключать участников может только владелец")
		}

		n, err := q.RemoveMember(ctx, store.RemoveMemberParams{HouseholdID: householdID, UserID: targetID})
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.NotFound("такого участника в семье нет")
		}

		counts, err := q.CountMembers(ctx, householdID)
		if err != nil {
			return err
		}
		switch {
		case counts.Members == 0:
			return q.DeleteHousehold(ctx, householdID)
		case counts.Owners == 0:
			return q.PromoteOldestMember(ctx, householdID)
		}
		return nil
	})
}

func (s *Service) CreateInvite(ctx context.Context, userID, householdID uuid.UUID, maxUses int, ttl time.Duration) (Invite, error) {
	if maxUses < 1 || maxUses > MaxInviteUses {
		return Invite{}, apperr.Invalid("maxUses", "число использований должно быть от 1 до 50")
	}
	if ttl < time.Hour || ttl > MaxInviteTTL {
		return Invite{}, apperr.Invalid("ttlHours", "срок действия должен быть от 1 часа до 30 дней")
	}
	if _, err := s.role(ctx, s.q, userID, householdID); err != nil {
		return Invite{}, err
	}
	active, err := s.q.ListActiveInvites(ctx, householdID)
	if err != nil {
		return Invite{}, err
	}
	if len(active) >= maxActiveInvites {
		return Invite{}, apperr.Conflict("too_many_invites", "слишком много действующих кодов, отзовите ненужные")
	}

	// A collision in a 31^8 space is astronomically unlikely, but retrying
	// is cheaper than reasoning about it.
	for range 3 {
		code, err := newInviteCode()
		if err != nil {
			return Invite{}, err
		}
		inv, err := s.q.CreateInvite(ctx, store.CreateInviteParams{
			Code:        code,
			HouseholdID: householdID,
			CreatedBy:   userID,
			MaxUses:     int32(maxUses),
			ExpiresAt:   time.Now().Add(ttl),
		})
		if isUniqueViolation(err) {
			continue
		}
		if err != nil {
			return Invite{}, err
		}
		creator, err := s.q.GetUserByID(ctx, userID)
		if err != nil {
			return Invite{}, err
		}
		return Invite{Code: inv.Code, MaxUses: int(inv.MaxUses), Uses: int(inv.Uses), ExpiresAt: inv.ExpiresAt, CreatedByName: creator.DisplayName}, nil
	}
	return Invite{}, errors.New("could not generate a unique invite code")
}

func (s *Service) ListInvites(ctx context.Context, userID, householdID uuid.UUID) ([]Invite, error) {
	if _, err := s.role(ctx, s.q, userID, householdID); err != nil {
		return nil, err
	}
	rows, err := s.q.ListActiveInvites(ctx, householdID)
	if err != nil {
		return nil, err
	}
	out := make([]Invite, len(rows))
	for i, r := range rows {
		out[i] = Invite{Code: r.Code, MaxUses: int(r.MaxUses), Uses: int(r.Uses), ExpiresAt: r.ExpiresAt, CreatedByName: r.CreatedByName}
	}
	return out, nil
}

func (s *Service) RevokeInvite(ctx context.Context, userID, householdID uuid.UUID, code string) error {
	if _, err := s.role(ctx, s.q, userID, householdID); err != nil {
		return err
	}
	n, err := s.q.RevokeInvite(ctx, store.RevokeInviteParams{HouseholdID: householdID, Code: NormalizeInviteCode(code)})
	if err != nil {
		return err
	}
	if n == 0 {
		return errInviteNotFound
	}
	return nil
}

func (s *Service) PreviewInvite(ctx context.Context, code string) (store.GetInvitePreviewRow, error) {
	row, err := s.q.GetInvitePreview(ctx, NormalizeInviteCode(code))
	if errors.Is(err, pgx.ErrNoRows) {
		return row, errInviteNotFound
	}
	return row, err
}

// AcceptInvite adds the user to the invite's household. Accepting a code for
// a household you're already in is a no-op and doesn't consume a use.
func (s *Service) AcceptInvite(ctx context.Context, userID uuid.UUID, code string) (Detail, error) {
	var householdID uuid.UUID
	var joined bool
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		inv, err := q.GetActiveInviteForUpdate(ctx, NormalizeInviteCode(code))
		if errors.Is(err, pgx.ErrNoRows) {
			return errInviteNotFound
		}
		if err != nil {
			return err
		}
		householdID = inv.HouseholdID
		if _, err := q.LockHousehold(ctx, householdID); err != nil {
			return err
		}

		counts, err := q.CountMembers(ctx, householdID)
		if err != nil {
			return err
		}
		added, err := q.AddMember(ctx, store.AddMemberParams{HouseholdID: householdID, UserID: userID, Role: RoleMember})
		if err != nil {
			return err
		}
		if added == 0 {
			return nil // already a member
		}
		if int(counts.Members) >= maxHouseholdMember {
			return apperr.Conflict("household_full", "в семье уже максимальное число участников")
		}
		joined = true
		return q.IncrementInviteUses(ctx, inv.Code)
	})
	if err != nil {
		return Detail{}, err
	}
	if joined {
		s.notify(householdID)
	}
	return s.Get(ctx, userID, householdID)
}

// role returns the user's role, or errHouseholdNotFound if they aren't a member.
func (s *Service) role(ctx context.Context, q *store.Queries, userID, householdID uuid.UUID) (string, error) {
	role, err := q.GetMemberRole(ctx, store.GetMemberRoleParams{HouseholdID: householdID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errHouseholdNotFound
	}
	return role, err
}

func getDetail(ctx context.Context, q *store.Queries, userID, householdID uuid.UUID) (Detail, error) {
	h, err := q.GetHouseholdForMember(ctx, store.GetHouseholdForMemberParams{HouseholdID: householdID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, errHouseholdNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	members, err := q.ListMembers(ctx, householdID)
	if err != nil {
		return Detail{}, err
	}
	return Detail{ID: h.ID, Name: h.Name, Role: h.Role, Members: members}, nil
}

func normalizeName(raw string) (string, error) {
	name := strings.Join(strings.Fields(raw), " ")
	if name == "" {
		return "", apperr.Invalid("name", "укажите название")
	}
	if utf8.RuneCountInString(name) > maxNameLen {
		return "", apperr.Invalid("name", "название должно быть не длиннее 60 символов")
	}
	return name, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
