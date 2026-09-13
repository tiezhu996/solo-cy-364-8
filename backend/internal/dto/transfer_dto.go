package dto

// TransferCreateRequest 调拨单创建请求（直接提交为待确认，需库存充足）。
type TransferCreateRequest struct {
	FromStoreID uint   `json:"from_store_id" binding:"required"`
	ToStoreID   uint   `json:"to_store_id" binding:"required"`
	SKUID       uint   `json:"sku_id" binding:"required"`
	Quantity    int    `json:"quantity" binding:"required,min=1"`
	Reason      string `json:"reason" binding:"max=255"`
}

// TransferDraftSaveRequest 调拨草稿暂存请求。
// 仅要求两门店不同、商品与数量有效，允许库存不足时保存。
type TransferDraftSaveRequest struct {
	FromStoreID uint   `json:"from_store_id" binding:"required"`
	ToStoreID   uint   `json:"to_store_id" binding:"required"`
	SKUID       uint   `json:"sku_id" binding:"required"`
	Quantity    int    `json:"quantity" binding:"required,min=1"`
	Reason      string `json:"reason" binding:"max=255"`
}

// TransferDraftUpdateRequest 调拨草稿编辑请求（仅创建人本人可改）。
type TransferDraftUpdateRequest struct {
	FromStoreID uint   `json:"from_store_id" binding:"required"`
	ToStoreID   uint   `json:"to_store_id" binding:"required"`
	SKUID       uint   `json:"sku_id" binding:"required"`
	Quantity    int    `json:"quantity" binding:"required,min=1"`
	Reason      string `json:"reason" binding:"max=255"`
}
