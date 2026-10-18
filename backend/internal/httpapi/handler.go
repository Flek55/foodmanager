// Package httpapi implements the HTTP layer: routing, middleware and the
// handlers behind the generated api.StrictServerInterface.
package httpapi

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"foodmanager/internal/api"
)

// Handler implements api.StrictServerInterface. The compiler check below
// fails the build if a new endpoint is added to openapi.yaml but not here.
type Handler struct {
	db      *pgxpool.Pool
	version string
}

var _ api.StrictServerInterface = (*Handler)(nil)

func NewHandler(db *pgxpool.Pool, version string) *Handler {
	return &Handler{db: db, version: version}
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
