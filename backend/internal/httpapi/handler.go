// Package httpapi implements the HTTP layer: routing, middleware and the
// handlers behind the generated api.StrictServerInterface. Handlers only
// translate between API types and services; business rules live in services.
package httpapi

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"foodmanager/internal/api"
	"foodmanager/internal/auth"
	"foodmanager/internal/events"
	"foodmanager/internal/household"
	"foodmanager/internal/shopping"
)

// Handler implements api.StrictServerInterface. The compiler check below
// fails the build if a new endpoint is added to openapi.yaml but not here.
type Handler struct {
	db         *pgxpool.Pool
	auth       *auth.Service
	households *household.Service
	shopping   *shopping.Service
	hub        *events.Hub
	cookies    cookieConfig
	limits     *rateLimits
	version    string
}

var _ api.StrictServerInterface = (*Handler)(nil)

type Deps struct {
	DB           *pgxpool.Pool
	Auth         *auth.Service
	Households   *household.Service
	Shopping     *shopping.Service
	Hub          *events.Hub
	SecureCookie bool
	Version      string
}

func NewHandler(d Deps) *Handler {
	return &Handler{
		db:         d.DB,
		auth:       d.Auth,
		households: d.Households,
		shopping:   d.Shopping,
		hub:        d.Hub,
		cookies:    cookieConfig{secure: d.SecureCookie},
		limits:     newRateLimits(),
		version:    d.Version,
	}
}

func (h *Handler) GetHealth(ctx context.Context, _ api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		return api.GetHealth503JSONResponse{
			Status:   api.HealthStatusDegraded,
			Database: api.HealthDatabaseUnavailable,
			Version:  h.version,
		}, nil
	}
	return api.GetHealth200JSONResponse{
		Status:   api.HealthStatusOk,
		Database: api.HealthDatabaseOk,
		Version:  h.version,
	}, nil
}
