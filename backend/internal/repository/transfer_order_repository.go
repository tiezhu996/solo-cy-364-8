package repository

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/ld/storeinventory/internal/constants"
	"github.com/ld/storeinventory/internal/model"
	"github.com/ld/storeinventory/internal/util"
)

// TransferOrderRepository 调拨单仓储。
type TransferOrderRepository interface {
	Create(order *model.TransferOrder) error
	FindByID(id uint) (*model.TransferOrder, error)
	List(page, pageSize int, storeID uint, status constants.TransferStatus) ([]model.TransferOrder, int64, error)
	ListDrafts(page, pageSize int, creatorID uint) ([]model.TransferOrder, int64, error)
	Update(order *model.TransferOrder) error
	UpdateTx(tx *gorm.DB, order *model.TransferOrder) error
	// UpdateDraftFieldsTx 仅当单据仍是本人草稿（id+creator_id+status=draft）时更新可编辑字段，
	// 不写 status/creator_id，避免并发的提交/作废已改状态后被整行覆盖回草稿。
	UpdateDraftFieldsTx(tx *gorm.DB, order *model.TransferOrder) error
	TransitionStatusTx(tx *gorm.DB, id uint, from, to constants.TransferStatus) error
}

type transferOrderRepository struct {
	db *gorm.DB
}

// NewTransferOrderRepository 构造调拨单仓储。
func NewTransferOrderRepository(db *gorm.DB) TransferOrderRepository {
	return &transferOrderRepository{db: db}
}

func (r *transferOrderRepository) Create(order *model.TransferOrder) error {
	if err := r.db.Create(order).Error; err != nil {
		return fmt.Errorf("create transfer order: %w", err)
	}
	return nil
}

func (r *transferOrderRepository) FindByID(id uint) (*model.TransferOrder, error) {
	var o model.TransferOrder
	err := r.db.Preload("FromStore").Preload("ToStore").Preload("SKU").First(&o, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("find transfer order by id: %w", util.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("find transfer order by id: %w", err)
	}
	return &o, nil
}

func (r *transferOrderRepository) List(page, pageSize int, storeID uint, status constants.TransferStatus) ([]model.TransferOrder, int64, error) {
	var orders []model.TransferOrder
	var total int64
	q := r.db.Model(&model.TransferOrder{})
	// 草稿及其作废终态不进入确认队列，普通列表（含默认待确认视图）一律排除。
	q = q.Where("status NOT IN ?", []constants.TransferStatus{constants.TransferDraft, constants.TransferVoided})
	if storeID > 0 {
		q = q.Where("from_store_id = ? OR to_store_id = ?", storeID, storeID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count transfer orders: %w", err)
	}
	if err := q.Preload("FromStore").Preload("ToStore").Preload("SKU").
		Offset((page - 1) * pageSize).Limit(pageSize).Order("id desc").Find(&orders).Error; err != nil {
		return nil, 0, fmt.Errorf("list transfer orders: %w", err)
	}
	return orders, total, nil
}

// ListDrafts 查询某店长（创建人）名下的草稿及其作废留痕（draft/voided），草稿在前，仅创建人本人可见。
func (r *transferOrderRepository) ListDrafts(page, pageSize int, creatorID uint) ([]model.TransferOrder, int64, error) {
	var orders []model.TransferOrder
	var total int64
	q := r.db.Model(&model.TransferOrder{}).
		Where("status IN ? AND creator_id = ?", []constants.TransferStatus{constants.TransferDraft, constants.TransferVoided}, creatorID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count transfer drafts: %w", err)
	}
	if err := q.Preload("FromStore").Preload("ToStore").Preload("SKU").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Order("status asc, updated_at desc, id desc").Find(&orders).Error; err != nil {
		return nil, 0, fmt.Errorf("list transfer drafts: %w", err)
	}
	return orders, total, nil
}

func (r *transferOrderRepository) Update(order *model.TransferOrder) error {
	return r.UpdateTx(nil, order)
}

func (r *transferOrderRepository) UpdateTx(tx *gorm.DB, order *model.TransferOrder) error {
	if err := dbOrTx(r.db, tx).Save(order).Error; err != nil {
		return fmt.Errorf("update transfer order: %w", err)
	}
	return nil
}

// UpdateDraftFieldsTx 条件更新草稿字段：仅当 id + creator_id + status=draft 仍匹配时生效，
// 只更新门店/商品/数量/原因，不回写 status（updated_at 由 GORM 自动刷新）。
func (r *transferOrderRepository) UpdateDraftFieldsTx(tx *gorm.DB, order *model.TransferOrder) error {
	res := dbOrTx(r.db, tx).Model(&model.TransferOrder{}).
		Where("id = ? AND creator_id = ? AND status = ?", order.ID, order.CreatorID, constants.TransferDraft).
		Updates(map[string]any{
			"from_store_id": order.FromStoreID,
			"to_store_id":   order.ToStoreID,
			"sku_id":        order.SKUID,
			"quantity":      order.Quantity,
			"reason":        order.Reason,
		})
	if res.Error != nil {
		return fmt.Errorf("update transfer draft fields: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// 单据不存在、非本人所有，或已被并发提交/作废（status 不再是 draft）。
		return fmt.Errorf("update transfer draft fields: %w", util.ErrConflict)
	}
	return nil
}

func (r *transferOrderRepository) TransitionStatusTx(tx *gorm.DB, id uint, from, to constants.TransferStatus) error {
	res := dbOrTx(r.db, tx).Model(&model.TransferOrder{}).
		Where("id = ? AND status = ?", id, from).
		Update("status", to)
	if res.Error != nil {
		return fmt.Errorf("transition transfer order status: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("transition transfer order status: %w", util.ErrConflict)
	}
	return nil
}
