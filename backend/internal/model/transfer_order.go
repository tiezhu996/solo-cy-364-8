package model

import (
	"time"

	"github.com/ld/storeinventory/internal/constants"
)

// TransferOrder 调拨单：调出/调入门店间 SKU 调拨，全程留痕。
// 草稿（status=draft）由创建人 CreatorID 店长持有，不进入确认队列，可经"提交"转为待确认。
type TransferOrder struct {
	ID          uint                     `gorm:"primaryKey" json:"id"`
	FromStoreID uint                     `gorm:"index;not null" json:"from_store_id"`
	FromStore   *Store                   `gorm:"foreignKey:FromStoreID" json:"from_store,omitempty"`
	ToStoreID   uint                     `gorm:"index;not null" json:"to_store_id"`
	ToStore     *Store                   `gorm:"foreignKey:ToStoreID" json:"to_store,omitempty"`
	SKUID       uint                     `gorm:"column:sku_id;index;not null" json:"sku_id"`
	SKU         *SKU                     `gorm:"foreignKey:SKUID" json:"sku,omitempty"`
	Quantity    int                      `gorm:"not null" json:"quantity"`
	Reason      string                   `gorm:"size:255" json:"reason"`
	Status      constants.TransferStatus `gorm:"size:16;index;not null" json:"status"`
	CreatorID   uint                     `gorm:"index;not null;default:0" json:"creator_id"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
}
