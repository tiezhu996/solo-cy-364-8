package handler

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ld/storeinventory/internal/constants"
	"github.com/ld/storeinventory/internal/dto"
	"github.com/ld/storeinventory/internal/middleware"
	"github.com/ld/storeinventory/internal/model"
	"github.com/ld/storeinventory/internal/service"
	"github.com/ld/storeinventory/internal/util"
)

// TransferOrderHandler 调拨单接口处理器。
type TransferOrderHandler struct {
	transferSvc service.TransferOrderService
}

// NewTransferOrderHandler 构造调拨单处理器。
func NewTransferOrderHandler(transferSvc service.TransferOrderService) *TransferOrderHandler {
	return &TransferOrderHandler{transferSvc: transferSvc}
}

// Create 发起调拨申请（直接提交为待确认，库存不足拒绝）。
func (h *TransferOrderHandler) Create(c *gin.Context) {
	var req dto.TransferCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(util.Validation(constants.MsgInvalidRequest, err))
		return
	}
	claims, err := middleware.CurrentUser(c)
	if err != nil {
		c.Error(util.Unauthorized(constants.MsgUnauthorized, err))
		return
	}
	order, err := h.transferSvc.Create(claims.UserID, req.FromStoreID, req.ToStoreID, req.SKUID, req.Quantity, req.Reason)
	if err != nil {
		c.Error(fmt.Errorf("handler create transfer: %w", err))
		return
	}
	util.OKMessage(c, constants.MsgTransferCreateSuccess, order)
}

// SaveDraft 暂存调拨草稿（允许库存不足）。
func (h *TransferOrderHandler) SaveDraft(c *gin.Context) {
	var req dto.TransferDraftSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(util.Validation(constants.MsgInvalidRequest, err))
		return
	}
	claims, err := middleware.CurrentUser(c)
	if err != nil {
		c.Error(util.Unauthorized(constants.MsgUnauthorized, err))
		return
	}
	order, err := h.transferSvc.SaveDraft(claims.UserID, req.FromStoreID, req.ToStoreID, req.SKUID, req.Quantity, req.Reason)
	if err != nil {
		c.Error(fmt.Errorf("handler save transfer draft: %w", err))
		return
	}
	util.OKMessage(c, constants.MsgTransferDraftSaved, order)
}

// ListDrafts 我的草稿列表（仅返回当前店长本人创建的草稿）。
func (h *TransferOrderHandler) ListDrafts(c *gin.Context) {
	var q dto.ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.Error(util.Validation(constants.MsgInvalidRequest, err))
		return
	}
	q.Normalize()
	claims, err := middleware.CurrentUser(c)
	if err != nil {
		c.Error(util.Unauthorized(constants.MsgUnauthorized, err))
		return
	}
	orders, total, err := h.transferSvc.ListDrafts(q.Page, q.PageSize, claims.UserID)
	if err != nil {
		c.Error(fmt.Errorf("handler list transfer drafts: %w", err))
		return
	}
	util.OK(c, gin.H{"list": orders, "total": total, "page": q.Page, "page_size": q.PageSize})
}

// UpdateDraft 编辑本人草稿。
func (h *TransferOrderHandler) UpdateDraft(c *gin.Context) {
	id, ok := parseTransferID(c)
	if !ok {
		return
	}
	var req dto.TransferDraftUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(util.Validation(constants.MsgInvalidRequest, err))
		return
	}
	claims, err := middleware.CurrentUser(c)
	if err != nil {
		c.Error(util.Unauthorized(constants.MsgUnauthorized, err))
		return
	}
	order, err := h.transferSvc.UpdateDraft(id, claims.UserID, req.FromStoreID, req.ToStoreID, req.SKUID, req.Quantity, req.Reason)
	if err != nil {
		c.Error(fmt.Errorf("handler update transfer draft: %w", err))
		return
	}
	util.OKMessage(c, constants.MsgTransferDraftUpdated, order)
}

// VoidDraft 作废本人草稿。
func (h *TransferOrderHandler) VoidDraft(c *gin.Context) {
	id, ok := parseTransferID(c)
	if !ok {
		return
	}
	claims, err := middleware.CurrentUser(c)
	if err != nil {
		c.Error(util.Unauthorized(constants.MsgUnauthorized, err))
		return
	}
	order, err := h.transferSvc.VoidDraft(id, claims.UserID)
	if err != nil {
		c.Error(fmt.Errorf("handler void transfer draft: %w", err))
		return
	}
	util.OKMessage(c, constants.MsgTransferDraftVoid, order)
}

// SubmitDraft 提交本人草稿：校验库存并转为待确认，库存不足保留草稿。
func (h *TransferOrderHandler) SubmitDraft(c *gin.Context) {
	id, ok := parseTransferID(c)
	if !ok {
		return
	}
	claims, err := middleware.CurrentUser(c)
	if err != nil {
		c.Error(util.Unauthorized(constants.MsgUnauthorized, err))
		return
	}
	order, err := h.transferSvc.SubmitDraft(id, claims.UserID)
	if err != nil {
		c.Error(fmt.Errorf("handler submit transfer draft: %w", err))
		return
	}
	util.OKMessage(c, constants.MsgTransferDraftSubmit, order)
}

// List 调拨单列表（草稿不进入该列表）。
func (h *TransferOrderHandler) List(c *gin.Context) {
	var q dto.ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.Error(util.Validation(constants.MsgInvalidRequest, err))
		return
	}
	q.Normalize()
	storeID, _ := strconv.ParseUint(c.Query("store_id"), 10, 64)
	status := constants.TransferStatus(c.Query("status"))
	orders, total, err := h.transferSvc.List(q.Page, q.PageSize, uint(storeID), status)
	if err != nil {
		c.Error(fmt.Errorf("handler list transfers: %w", err))
		return
	}
	util.OK(c, gin.H{"list": orders, "total": total, "page": q.Page, "page_size": q.PageSize})
}

func parseTransferID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(util.Validation("无效的调拨单ID", err))
		return 0, false
	}
	return uint(id), true
}

func (h *TransferOrderHandler) transition(c *gin.Context, action func(uint) (*model.TransferOrder, error), successMsg string) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(util.Validation("无效的调拨单ID", err))
		return
	}
	order, err := action(uint(id))
	if err != nil {
		c.Error(fmt.Errorf("handler transfer transition: %w", err))
		return
	}
	util.OKMessage(c, successMsg, order)
}

// Confirm 审批确认。
func (h *TransferOrderHandler) Confirm(c *gin.Context) {
	h.transition(c, h.transferSvc.Confirm, constants.MsgTransferConfirmSuccess)
}

// Ship 发货。
func (h *TransferOrderHandler) Ship(c *gin.Context) {
	h.transition(c, h.transferSvc.Ship, constants.MsgTransferShipSuccess)
}

// Receive 确认收货。
func (h *TransferOrderHandler) Receive(c *gin.Context) {
	h.transition(c, h.transferSvc.Receive, constants.MsgTransferReceiveSuccess)
}

// Cancel 取消。
func (h *TransferOrderHandler) Cancel(c *gin.Context) {
	h.transition(c, h.transferSvc.Cancel, constants.MsgTransferCancelSuccess)
}
