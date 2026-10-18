package httpapi

import (
	"context"
	"time"

	"foodmanager/internal/api"
	"foodmanager/internal/apperr"
	"foodmanager/internal/household"
)

func (h *Handler) CreateHousehold(ctx context.Context, req api.CreateHouseholdRequestObject) (api.CreateHouseholdResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	d, err := h.households.Create(ctx, user.ID, req.Body.Name)
	if err != nil {
		return nil, err
	}
	return api.CreateHousehold201JSONResponse(toHouseholdDetail(d)), nil
}

func (h *Handler) GetHousehold(ctx context.Context, req api.GetHouseholdRequestObject) (api.GetHouseholdResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.households.Get(ctx, user.ID, req.HouseholdId)
	if err != nil {
		return nil, err
	}
	return api.GetHousehold200JSONResponse(toHouseholdDetail(d)), nil
}

func (h *Handler) UpdateHousehold(ctx context.Context, req api.UpdateHouseholdRequestObject) (api.UpdateHouseholdResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	d, err := h.households.Rename(ctx, user.ID, req.HouseholdId, req.Body.Name)
	if err != nil {
		return nil, err
	}
	return api.UpdateHousehold200JSONResponse(toHouseholdDetail(d)), nil
}

func (h *Handler) RemoveMember(ctx context.Context, req api.RemoveMemberRequestObject) (api.RemoveMemberResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.households.RemoveMember(ctx, user.ID, req.HouseholdId, req.UserId); err != nil {
		return nil, err
	}
	return api.RemoveMember204Response{}, nil
}

func (h *Handler) ListInvites(ctx context.Context, req api.ListInvitesRequestObject) (api.ListInvitesResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	invites, err := h.households.ListInvites(ctx, user.ID, req.HouseholdId)
	if err != nil {
		return nil, err
	}
	out := make(api.ListInvites200JSONResponse, len(invites))
	for i, inv := range invites {
		out[i] = toInvite(inv)
	}
	return out, nil
}

func (h *Handler) CreateInvite(ctx context.Context, req api.CreateInviteRequestObject) (api.CreateInviteResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	maxUses, ttl := household.DefaultInviteUses, household.DefaultInviteTTL
	if req.Body != nil {
		if req.Body.MaxUses != nil {
			maxUses = *req.Body.MaxUses
		}
		if req.Body.TtlHours != nil {
			ttl = time.Duration(*req.Body.TtlHours) * time.Hour
		}
	}
	inv, err := h.households.CreateInvite(ctx, user.ID, req.HouseholdId, maxUses, ttl)
	if err != nil {
		return nil, err
	}
	return api.CreateInvite201JSONResponse(toInvite(inv)), nil
}

func (h *Handler) RevokeInvite(ctx context.Context, req api.RevokeInviteRequestObject) (api.RevokeInviteResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.households.RevokeInvite(ctx, user.ID, req.HouseholdId, req.Code); err != nil {
		return nil, err
	}
	return api.RevokeInvite204Response{}, nil
}

func (h *Handler) PreviewInvite(ctx context.Context, req api.PreviewInviteRequestObject) (api.PreviewInviteResponseObject, error) {
	p, err := h.households.PreviewInvite(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	return api.PreviewInvite200JSONResponse{
		HouseholdName: p.HouseholdName,
		InvitedBy:     p.InvitedBy,
		MemberCount:   int(p.MemberCount),
	}, nil
}

func (h *Handler) AcceptInvite(ctx context.Context, req api.AcceptInviteRequestObject) (api.AcceptInviteResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.households.AcceptInvite(ctx, user.ID, req.Code)
	if err != nil {
		return nil, err
	}
	return api.AcceptInvite200JSONResponse(toHouseholdDetail(d)), nil
}

func toHouseholdDetail(d household.Detail) api.HouseholdDetail {
	members := make([]api.Member, len(d.Members))
	for i, m := range d.Members {
		members[i] = api.Member{
			UserId:      m.UserID,
			DisplayName: m.DisplayName,
			AvatarUrl:   m.AvatarUrl,
			Role:        api.Role(m.Role),
			JoinedAt:    m.JoinedAt,
		}
	}
	return api.HouseholdDetail{Id: d.ID, Name: d.Name, Role: api.Role(d.Role), Members: members}
}

func toInvite(inv household.Invite) api.Invite {
	return api.Invite{
		Code:      household.FormatInviteCode(inv.Code),
		MaxUses:   inv.MaxUses,
		Uses:      inv.Uses,
		ExpiresAt: inv.ExpiresAt,
		CreatedBy: inv.CreatedByName,
	}
}
