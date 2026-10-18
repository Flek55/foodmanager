package httpapi

import (
	"context"

	"foodmanager/internal/api"
	"foodmanager/internal/apperr"
	"foodmanager/internal/shopping"
	"foodmanager/internal/store"
)

func (h *Handler) GetList(ctx context.Context, req api.GetListRequestObject) (api.GetListResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	snap, err := h.shopping.Snapshot(ctx, user.ID, req.HouseholdId)
	if err != nil {
		return nil, err
	}
	out := api.ListSnapshot{
		Version:    snap.Version,
		Items:      make([]api.ListItem, len(snap.Items)),
		Products:   make([]api.Product, len(snap.Products)),
		Categories: make([]api.Category, len(snap.Categories)),
		Stores:     make([]api.Store, len(snap.Stores)),
	}
	for i, it := range snap.Items {
		out.Items[i] = toListItem(it)
	}
	for i, p := range snap.Products {
		out.Products[i] = api.Product{
			Id: p.ID, Name: p.Name, Description: p.Description, CategoryId: p.CategoryID,
			DefaultUnit: toUnit(p.DefaultUnit), DefaultStoreId: p.DefaultStoreID, UseCount: int(p.UseCount),
		}
	}
	for i, c := range snap.Categories {
		out.Categories[i] = toCategory(c)
	}
	for i, s := range snap.Stores {
		out.Stores[i] = toStore(s)
	}
	return api.GetList200JSONResponse(out), nil
}

func (h *Handler) AddItem(ctx context.Context, req api.AddItemRequestObject) (api.AddItemResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	b := req.Body
	item, created, err := h.shopping.AddItem(ctx, user.ID, req.HouseholdId, shopping.AddItemInput{
		ID: b.Id, Name: b.Name, Quantity: b.Quantity, Unit: fromUnit(b.Unit),
		Note: b.Note, StoreID: b.StoreId, CategoryID: b.CategoryId,
	})
	if err != nil {
		return nil, err
	}
	if created {
		return api.AddItem201JSONResponse(toListItem(item)), nil
	}
	return api.AddItem200JSONResponse(toListItem(item)), nil
}

func (h *Handler) UpdateItem(ctx context.Context, req api.UpdateItemRequestObject) (api.UpdateItemResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	b := req.Body
	item, err := h.shopping.UpdateItem(ctx, user.ID, req.HouseholdId, req.ItemId, shopping.UpdateItemInput{
		Name: b.Name, Description: b.Description, CategoryID: b.CategoryId,
		Quantity: b.Quantity, Unit: fromUnit(b.Unit), Note: b.Note, StoreID: b.StoreId,
	})
	if err != nil {
		return nil, err
	}
	return api.UpdateItem200JSONResponse(toListItem(item)), nil
}

func (h *Handler) SetItemChecked(ctx context.Context, req api.SetItemCheckedRequestObject) (api.SetItemCheckedResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	item, err := h.shopping.SetChecked(ctx, user.ID, req.HouseholdId, req.ItemId, req.Body.Checked)
	if err != nil {
		return nil, err
	}
	return api.SetItemChecked200JSONResponse(toListItem(item)), nil
}

func (h *Handler) DeleteItem(ctx context.Context, req api.DeleteItemRequestObject) (api.DeleteItemResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.shopping.DeleteItem(ctx, user.ID, req.HouseholdId, req.ItemId); err != nil {
		return nil, err
	}
	return api.DeleteItem204Response{}, nil
}

func (h *Handler) ClearCheckedItems(ctx context.Context, req api.ClearCheckedItemsRequestObject) (api.ClearCheckedItemsResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	n, err := h.shopping.ClearChecked(ctx, user.ID, req.HouseholdId)
	if err != nil {
		return nil, err
	}
	return api.ClearCheckedItems200JSONResponse{Removed: int(n)}, nil
}

func (h *Handler) CreateStore(ctx context.Context, req api.CreateStoreRequestObject) (api.CreateStoreResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	st, err := h.shopping.CreateStore(ctx, user.ID, req.HouseholdId, req.Body.Name)
	if err != nil {
		return nil, err
	}
	return api.CreateStore201JSONResponse(toStore(st)), nil
}

func (h *Handler) UpdateStore(ctx context.Context, req api.UpdateStoreRequestObject) (api.UpdateStoreResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	st, err := h.shopping.RenameStore(ctx, user.ID, req.HouseholdId, req.StoreId, req.Body.Name)
	if err != nil {
		return nil, err
	}
	return api.UpdateStore200JSONResponse(toStore(st)), nil
}

func (h *Handler) DeleteStore(ctx context.Context, req api.DeleteStoreRequestObject) (api.DeleteStoreResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.shopping.DeleteStore(ctx, user.ID, req.HouseholdId, req.StoreId); err != nil {
		return nil, err
	}
	return api.DeleteStore204Response{}, nil
}

func (h *Handler) CreateCategory(ctx context.Context, req api.CreateCategoryRequestObject) (api.CreateCategoryResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	c, err := h.shopping.CreateCategory(ctx, user.ID, req.HouseholdId, req.Body.Name, req.Body.Emoji)
	if err != nil {
		return nil, err
	}
	return api.CreateCategory201JSONResponse(toCategory(c)), nil
}

func (h *Handler) UpdateCategory(ctx context.Context, req api.UpdateCategoryRequestObject) (api.UpdateCategoryResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	c, err := h.shopping.UpdateCategory(ctx, user.ID, req.HouseholdId, req.CategoryId, req.Body.Name, req.Body.Emoji)
	if err != nil {
		return nil, err
	}
	return api.UpdateCategory200JSONResponse(toCategory(c)), nil
}

func (h *Handler) DeleteCategory(ctx context.Context, req api.DeleteCategoryRequestObject) (api.DeleteCategoryResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.shopping.DeleteCategory(ctx, user.ID, req.HouseholdId, req.CategoryId); err != nil {
		return nil, err
	}
	return api.DeleteCategory204Response{}, nil
}

func (h *Handler) ReorderCategories(ctx context.Context, req api.ReorderCategoriesRequestObject) (api.ReorderCategoriesResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	cats, err := h.shopping.ReorderCategories(ctx, user.ID, req.HouseholdId, req.Body.Ids)
	if err != nil {
		return nil, err
	}
	out := make(api.ReorderCategories200JSONResponse, len(cats))
	for i, c := range cats {
		out[i] = toCategory(c)
	}
	return out, nil
}

func toListItem(it store.ListItemViewsRow) api.ListItem {
	return api.ListItem{
		Id:          it.ID,
		ProductId:   it.ProductID,
		Name:        it.Name,
		Description: it.Description,
		CategoryId:  it.CategoryID,
		Quantity:    it.Quantity,
		Unit:        toUnit(it.Unit),
		Note:        it.Note,
		StoreId:     it.StoreID,
		AddedBy:     it.AddedByName,
		CreatedAt:   it.CreatedAt,
		CheckedAt:   it.CheckedAt,
		CheckedBy:   it.CheckedByName,
	}
}

func toCategory(c store.Category) api.Category {
	return api.Category{Id: c.ID, Name: c.Name, Emoji: c.Emoji, SortOrder: int(c.SortOrder), Key: c.Key}
}

func toStore(s store.Store) api.Store {
	return api.Store{Id: s.ID, Name: s.Name, SortOrder: int(s.SortOrder)}
}

func toUnit(u *string) *api.Unit {
	if u == nil {
		return nil
	}
	v := api.Unit(*u)
	return &v
}

func fromUnit(u *api.Unit) *string {
	if u == nil {
		return nil
	}
	v := string(*u)
	return &v
}
