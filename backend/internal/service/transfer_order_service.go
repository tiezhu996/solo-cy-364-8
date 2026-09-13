package service

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/ld/storeinventory/internal/constants"
	"github.com/ld/storeinventory/internal/model"
	"github.com/ld/storeinventory/internal/repository"
	"github.com/ld/storeinventory/internal/util"
)

// TransferOrderService 调拨单业务逻辑。
type TransferOrderService interface {
	Create(creatorID, fromStoreID, toStoreID, skuID uint, quantity int, reason string) (*model.TransferOrder, error)
	List(page, pageSize int, storeID uint, status constants.TransferStatus) ([]model.TransferOrder, int64, error)
	Confirm(id uint) (*model.TransferOrder, error)
	Ship(id uint) (*model.TransferOrder, error)
	Receive(id uint) (*model.TransferOrder, error)
	Cancel(id uint) (*model.TransferOrder, error)
	// 草稿环节
	SaveDraft(creatorID, fromStoreID, toStoreID, skuID uint, quantity int, reason string) (*model.TransferOrder, error)
	ListDrafts(page, pageSize int, creatorID uint) ([]model.TransferOrder, int64, error)
	UpdateDraft(id, creatorID, fromStoreID, toStoreID, skuID uint, quantity int, reason string) (*model.TransferOrder, error)
	VoidDraft(id, creatorID uint) (*model.TransferOrder, error)
	SubmitDraft(id, creatorID uint) (*model.TransferOrder, error)
}

type transferOrderService struct {
	orderRepo repository.TransferOrderRepository
	storeRepo repository.StoreRepository
	skuRepo   repository.SKURepository
	invSvc    StoreInventoryService
	recSvc    StockRecordService
	db        *gorm.DB
	logger    *slog.Logger
}

// NewTransferOrderService 构造调拨单服务。
func NewTransferOrderService(orderRepo repository.TransferOrderRepository, storeRepo repository.StoreRepository, skuRepo repository.SKURepository, invSvc StoreInventoryService, recSvc StockRecordService, db *gorm.DB, logger *slog.Logger) TransferOrderService {
	return &transferOrderService{orderRepo: orderRepo, storeRepo: storeRepo, skuRepo: skuRepo, invSvc: invSvc, recSvc: recSvc, db: db, logger: logger}
}

func (s *transferOrderService) Create(creatorID, fromStoreID, toStoreID, skuID uint, quantity int, reason string) (*model.TransferOrder, error) {
	if fromStoreID == toStoreID {
		return nil, fmt.Errorf("create transfer from[%d] to[%d] same store: %w", fromStoreID, toStoreID, util.ErrValidation)
	}
	if quantity <= 0 {
		return nil, fmt.Errorf("create transfer quantity[%d]: %w", quantity, util.ErrValidation)
	}
	if err := s.checkStoresAndSKU(fromStoreID, toStoreID, skuID); err != nil {
		return nil, fmt.Errorf("create transfer: %w", err)
	}
	if err := s.invSvc.CheckSufficient(fromStoreID, skuID, quantity); err != nil {
		return nil, fmt.Errorf("create transfer from[%d] sku[%d]: %w", fromStoreID, skuID, err)
	}
	order := &model.TransferOrder{
		FromStoreID: fromStoreID, ToStoreID: toStoreID, SKUID: skuID,
		Quantity: quantity, Reason: reason, Status: constants.TransferPending, CreatorID: creatorID,
	}
	if err := s.orderRepo.Create(order); err != nil {
		return nil, fmt.Errorf("create transfer: %w", err)
	}
	s.logger.Info(constants.LogTransferCreateSuccess, "order_id", order.ID, "creator", creatorID, "from", fromStoreID, "to", toStoreID, "sku", skuID, "qty", quantity)
	return order, nil
}

// SaveDraft 暂存草稿：校验两门店不同、商品与数量有效，但不校验库存；草稿不进入确认队列。
func (s *transferOrderService) SaveDraft(creatorID, fromStoreID, toStoreID, skuID uint, quantity int, reason string) (*model.TransferOrder, error) {
	if err := s.validateDraftFields(fromStoreID, toStoreID, skuID, quantity); err != nil {
		return nil, fmt.Errorf("save draft creator[%d]: %w", creatorID, err)
	}
	order := &model.TransferOrder{
		FromStoreID: fromStoreID, ToStoreID: toStoreID, SKUID: skuID,
		Quantity: quantity, Reason: reason, Status: constants.TransferDraft, CreatorID: creatorID,
	}
	if err := s.orderRepo.Create(order); err != nil {
		return nil, fmt.Errorf("save transfer draft: %w", err)
	}
	s.logger.Info(constants.LogTransferDraftSaved, "order_id", order.ID, "creator", creatorID, "from", fromStoreID, "to", toStoreID, "sku", skuID, "qty", quantity)
	return order, nil
}

func (s *transferOrderService) ListDrafts(page, pageSize int, creatorID uint) ([]model.TransferOrder, int64, error) {
	orders, total, err := s.orderRepo.ListDrafts(page, pageSize, creatorID)
	if err != nil {
		return nil, 0, fmt.Errorf("list transfer drafts creator[%d]: %w", creatorID, err)
	}
	s.logger.Info(constants.LogTransferDraftList, "creator", creatorID, "total", total)
	return orders, total, nil
}

// UpdateDraft 编辑草稿，仅草稿创建人本人可操作；同样允许库存不足时保存。
// 使用带状态条件的原子更新：若在加载后被并发的提交/作废先改变状态，则本次保存命中 0 行并返回冲突，
// 不会把已提交/已作废的单据覆盖回草稿。
func (s *transferOrderService) UpdateDraft(id, creatorID, fromStoreID, toStoreID, skuID uint, quantity int, reason string) (*model.TransferOrder, error) {
	order, err := s.loadOwnedDraft(id, creatorID)
	if err != nil {
		return nil, err
	}
	if err := s.validateDraftFields(fromStoreID, toStoreID, skuID, quantity); err != nil {
		return nil, fmt.Errorf("update draft[id=%d] creator[%d]: %w", id, creatorID, err)
	}
	order.FromStoreID = fromStoreID
	order.ToStoreID = toStoreID
	order.SKUID = skuID
	order.Quantity = quantity
	order.Reason = reason
	order.CreatorID = creatorID
	if err := s.orderRepo.UpdateDraftFieldsTx(nil, order); err != nil {
		return nil, fmt.Errorf("update draft[id=%d] creator[%d]: %w", id, creatorID, err)
	}
	s.logger.Info(constants.LogTransferDraftUpdated, "order_id", id, "creator", creatorID, "from", fromStoreID, "to", toStoreID, "sku", skuID, "qty", quantity)
	return order, nil
}

// VoidDraft 作废草稿，仅草稿创建人本人可操作，落到独立终态 voided，不进入共享的待确认/已取消队列。
func (s *transferOrderService) VoidDraft(id, creatorID uint) (*model.TransferOrder, error) {
	order, err := s.loadOwnedDraft(id, creatorID)
	if err != nil {
		return nil, err
	}
	// 单条带旧状态条件的更新本身是原子的，草稿作废不涉及库存联动，无需包裹事务。
	if err := s.orderRepo.TransitionStatusTx(nil, id, constants.TransferDraft, constants.TransferVoided); err != nil {
		return nil, fmt.Errorf("void draft[id=%d]: %w", id, err)
	}
	order.Status = constants.TransferVoided
	s.logger.Info(constants.LogTransferDraftVoided, "order_id", id, "creator", creatorID)
	return order, nil
}

// SubmitDraft 提交草稿：此时才校验库存，充足则转为待确认；库存不足则保留草稿并返回错误。
func (s *transferOrderService) SubmitDraft(id, creatorID uint) (*model.TransferOrder, error) {
	order, err := s.loadOwnedDraft(id, creatorID)
	if err != nil {
		return nil, err
	}
	if err := s.checkStoresAndSKU(order.FromStoreID, order.ToStoreID, order.SKUID); err != nil {
		return nil, fmt.Errorf("submit draft[id=%d]: %w", id, err)
	}
	// 提交时才校验库存；不足直接返回，草稿原样保留。
	if err := s.invSvc.CheckSufficient(order.FromStoreID, order.SKUID, order.Quantity); err != nil {
		s.logger.Warn(constants.LogTransferDraftSubmitFail, "order_id", id, "creator", creatorID, "error", err)
		return nil, fmt.Errorf("submit draft[id=%d] from[%d] sku[%d]: %w", id, order.FromStoreID, order.SKUID, err)
	}
	if err := s.orderRepo.TransitionStatusTx(nil, id, constants.TransferDraft, constants.TransferPending); err != nil {
		return nil, fmt.Errorf("submit draft[id=%d]: %w", id, err)
	}
	order.Status = constants.TransferPending
	s.logger.Info(constants.LogTransferDraftSubmitted, "order_id", id, "creator", creatorID)
	return order, nil
}

// validateDraftFields 草稿暂存/编辑校验：两门店不能相同、数量有效，不校验库存。
func (s *transferOrderService) validateDraftFields(fromStoreID, toStoreID, skuID uint, quantity int) error {
	if fromStoreID == 0 || toStoreID == 0 {
		return fmt.Errorf("store required from[%d] to[%d]: %w", fromStoreID, toStoreID, util.ErrValidation)
	}
	if fromStoreID == toStoreID {
		return fmt.Errorf("from[%d] to[%d] same store: %w", fromStoreID, toStoreID, util.ErrValidation)
	}
	if skuID == 0 {
		return fmt.Errorf("sku required sku[%d]: %w", skuID, util.ErrValidation)
	}
	if quantity <= 0 {
		return fmt.Errorf("quantity invalid qty[%d]: %w", quantity, util.ErrValidation)
	}
	return s.checkStoresAndSKU(fromStoreID, toStoreID, skuID)
}

// checkStoresAndSKU 校验调出/调入门店与商品均真实存在。
func (s *transferOrderService) checkStoresAndSKU(fromStoreID, toStoreID, skuID uint) error {
	if _, err := s.storeRepo.FindByID(fromStoreID); err != nil {
		return fmt.Errorf("from store[%d]: %w", fromStoreID, err)
	}
	if _, err := s.storeRepo.FindByID(toStoreID); err != nil {
		return fmt.Errorf("to store[%d]: %w", toStoreID, err)
	}
	if _, err := s.skuRepo.FindByID(skuID); err != nil {
		return fmt.Errorf("sku[%d]: %w", skuID, err)
	}
	return nil
}

// loadOwnedDraft 加载草稿并强制校验归属：草稿仅创建人本人可编辑/作废/提交，其他角色不可操作。
func (s *transferOrderService) loadOwnedDraft(id, creatorID uint) (*model.TransferOrder, error) {
	order, err := s.orderRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("load draft[id=%d]: %w", id, err)
	}
	if order.CreatorID != creatorID {
		s.logger.Warn(constants.LogTransferDraftDenied, "order_id", id, "creator", order.CreatorID, "operator", creatorID)
		return nil, fmt.Errorf("operator[%d] %s draft[id=%d] owner[%d]: %w", creatorID, "operate", id, order.CreatorID, util.ErrForbidden)
	}
	if order.Status != constants.TransferDraft {
		s.logger.Warn(constants.LogTransferStatusInvalid, "order_id", id, "status", order.Status)
		return nil, fmt.Errorf("draft[id=%d] status[%s]: %w", id, order.Status, util.ErrConflict)
	}
	return order, nil
}

func (s *transferOrderService) List(page, pageSize int, storeID uint, status constants.TransferStatus) ([]model.TransferOrder, int64, error) {
	orders, total, err := s.orderRepo.List(page, pageSize, storeID, status)
	if err != nil {
		return nil, 0, fmt.Errorf("list transfers: %w", err)
	}
	s.logger.Info(constants.LogTransferListQueried, "total", total)
	return orders, total, nil
}

func (s *transferOrderService) Confirm(id uint) (*model.TransferOrder, error) {
	order, err := s.orderRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("confirm transfer[id=%d]: %w", id, err)
	}
	if !constants.CanTransfer(order.Status, constants.TransferConfirmed) {
		s.logger.Warn(constants.LogTransferStatusInvalid, "order_id", id, "status", order.Status, "action", "confirm")
		return nil, fmt.Errorf("confirm transfer[id=%d] status[%s]: %w", id, order.Status, util.ErrConflict)
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.orderRepo.TransitionStatusTx(tx, id, order.Status, constants.TransferConfirmed); err != nil {
			return fmt.Errorf("confirm transfer[id=%d] status[%s]: %w", id, order.Status, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	order.Status = constants.TransferConfirmed
	s.logger.Info(constants.LogTransferConfirmSuccess, "order_id", id)
	return order, nil
}

func (s *transferOrderService) Ship(id uint) (*model.TransferOrder, error) {
	order, err := s.orderRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("ship transfer[id=%d]: %w", id, err)
	}
	if !constants.CanTransfer(order.Status, constants.TransferShipped) {
		s.logger.Warn(constants.LogTransferStatusInvalid, "order_id", id, "status", order.Status, "action", "ship")
		return nil, fmt.Errorf("ship transfer[id=%d] status[%s]: %w", id, order.Status, util.ErrConflict)
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.invSvc.CheckSufficientTx(tx, order.FromStoreID, order.SKUID, order.Quantity); err != nil {
			return fmt.Errorf("ship transfer[id=%d]: %w", id, err)
		}
		if err := s.invSvc.AdjustQuantityTx(tx, order.FromStoreID, order.SKUID, -order.Quantity); err != nil {
			return fmt.Errorf("ship transfer[id=%d]: %w", id, err)
		}
		if err := s.orderRepo.TransitionStatusTx(tx, id, order.Status, constants.TransferShipped); err != nil {
			return fmt.Errorf("ship transfer[id=%d] status[%s]: %w", id, order.Status, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	order.Status = constants.TransferShipped
	s.logger.Info(constants.LogTransferShipSuccess, "order_id", id, "from_store", order.FromStoreID, "sku", order.SKUID, "qty", order.Quantity)
	return order, nil
}

func (s *transferOrderService) Receive(id uint) (*model.TransferOrder, error) {
	order, err := s.orderRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("receive transfer[id=%d]: %w", id, err)
	}
	if order.Status != constants.TransferShipped {
		s.logger.Warn(constants.LogTransferStatusInvalid, "order_id", id, "status", order.Status, "action", "receive")
		return nil, fmt.Errorf("receive transfer[id=%d] status[%s]: %w", id, order.Status, util.ErrConflict)
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if _, err := s.invSvc.EnsureTx(tx, order.ToStoreID, order.SKUID, 0); err != nil {
			return fmt.Errorf("receive transfer[id=%d]: %w", id, err)
		}
		if _, err := s.recSvc.CreateTx(tx, order.ToStoreID, order.SKUID, constants.RecordTransferIn, order.Quantity, &id); err != nil {
			return fmt.Errorf("receive transfer[id=%d] create record: %w", id, err)
		}
		if err := s.orderRepo.TransitionStatusTx(tx, id, order.Status, constants.TransferReceived); err != nil {
			return fmt.Errorf("receive transfer[id=%d] status[%s]: %w", id, order.Status, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	order.Status = constants.TransferReceived
	s.logger.Info(constants.LogTransferReceiveSuccess, "order_id", id, "to_store", order.ToStoreID, "qty", order.Quantity)
	return order, nil
}

func (s *transferOrderService) Cancel(id uint) (*model.TransferOrder, error) {
	order, err := s.orderRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("cancel transfer[id=%d]: %w", id, err)
	}
	if !constants.CanTransfer(order.Status, constants.TransferCancelled) {
		s.logger.Warn(constants.LogTransferStatusInvalid, "order_id", id, "status", order.Status, "action", "cancel")
		return nil, fmt.Errorf("cancel transfer[id=%d] status[%s]: %w", id, order.Status, util.ErrConflict)
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.orderRepo.TransitionStatusTx(tx, id, order.Status, constants.TransferCancelled); err != nil {
			return fmt.Errorf("cancel transfer[id=%d] status[%s]: %w", id, order.Status, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	order.Status = constants.TransferCancelled
	s.logger.Info(constants.LogTransferCancelSuccess, "order_id", id)
	return order, nil
}
