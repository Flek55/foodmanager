package httpapi

import (
	"context"

	"foodmanager/internal/api"
	"foodmanager/internal/apperr"
	"foodmanager/internal/shopping"
	"foodmanager/internal/store"
)

func (h *Handler) SetStock(ctx context.Context, req api.SetStockRequestObject) (api.SetStockResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	res, err := h.shopping.SetStock(ctx, user.ID, req.HouseholdId, req.ProductId, string(req.Body.Status), req.Body.ItemId)
	if err != nil {
		return nil, err
	}
	return api.SetStock200JSONResponse(toStockResult(res)), nil
}

func (h *Handler) TrackProduct(ctx context.Context, req api.TrackProductRequestObject) (api.TrackProductResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	b := req.Body
	res, err := h.shopping.TrackProduct(ctx, user.ID, req.HouseholdId, b.Name, string(b.Status), b.ProductId, b.ItemId)
	if err != nil {
		return nil, err
	}
	return api.TrackProduct200JSONResponse(toStockResult(res)), nil
}

func (h *Handler) ListPurchases(ctx context.Context, req api.ListPurchasesRequestObject) (api.ListPurchasesResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.shopping.Purchases(ctx, user.ID, req.HouseholdId, req.ProductId)
	if err != nil {
		return nil, err
	}
	out := make(api.ListPurchases200JSONResponse, len(rows))
	for i, p := range rows {
		out[i] = api.Purchase{
			Id: p.ID, BoughtAt: p.BoughtAt, BoughtBy: p.BoughtByName,
			Quantity: p.Quantity, Unit: toUnit(p.Unit), StoreId: p.StoreID,
		}
	}
	return out, nil
}

func toStockResult(res shopping.StockResult) api.StockResult {
	out := api.StockResult{Product: toProduct(res.Product)}
	if res.Item != nil {
		item := toListItem(*res.Item)
		out.Item = &item
	}
	return out
}

func toProduct(p store.Product) api.Product {
	var status *api.StockStatus
	if p.StockStatus != nil {
		s := api.StockStatus(*p.StockStatus)
		status = &s
	}
	return api.Product{
		Id:              p.ID,
		Name:            p.Name,
		Description:     p.Description,
		CategoryId:      p.CategoryID,
		DefaultUnit:     toUnit(p.DefaultUnit),
		DefaultStoreId:  p.DefaultStoreID,
		UseCount:        int(p.UseCount),
		StockStatus:     status,
		StockUpdatedAt:  p.StockUpdatedAt,
		PurchaseCount:   int(p.PurchaseCount),
		LastPurchasedAt: p.LastPurchasedAt,
	}
}
