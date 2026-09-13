package service

import (
	"errors"
	"log/slog"
	"os"
	"testing"

	"gorm.io/gorm"

	"github.com/ld/storeinventory/internal/constants"
	"github.com/ld/storeinventory/internal/model"
	"github.com/ld/storeinventory/internal/util"
)

// mockStoreRepo 内存版门店仓储，仅供草稿 service 单测。
type mockStoreRepo struct{ ids map[uint]bool }

func (m *mockStoreRepo) Create(s *model.Store) error { m.ids[s.ID] = true; return nil }
func (m *mockStoreRepo) FindByID(id uint) (*model.Store, error) {
	if !m.ids[id] {
		return nil, util.ErrNotFound
	}
	return &model.Store{ID: id, Name: "store"}, nil
}
func (m *mockStoreRepo) FindByCode(code string) (*model.Store, error) { return nil, util.ErrNotFound }
func (m *mockStoreRepo) List(page, pageSize int) ([]model.Store, int64, error) {
	return nil, 0, nil
}
func (m *mockStoreRepo) ListAll() ([]model.Store, error) { return nil, nil }
func (m *mockStoreRepo) Update(s *model.Store) error     { return nil }
func (m *mockStoreRepo) Delete(id uint) error            { return nil }

// mockTransferOrderRepo 内存版调拨单仓储，支持草稿测试。
type mockTransferOrderRepo struct {
	items map[uint]*model.TransferOrder
	seq   uint
}

func newMockTransferOrderRepo() *mockTransferOrderRepo {
	return &mockTransferOrderRepo{items: make(map[uint]*model.TransferOrder)}
}

func (m *mockTransferOrderRepo) Create(o *model.TransferOrder) error {
	m.seq++
	o.ID = m.seq
	cp := *o
	m.items[o.ID] = &cp
	return nil
}
func (m *mockTransferOrderRepo) FindByID(id uint) (*model.TransferOrder, error) {
	o, ok := m.items[id]
	if !ok {
		return nil, util.ErrNotFound
	}
	cp := *o
	return &cp, nil
}
func (m *mockTransferOrderRepo) List(page, pageSize int, storeID uint, status constants.TransferStatus) ([]model.TransferOrder, int64, error) {
	var out []model.TransferOrder
	for _, o := range m.items {
		if o.Status == constants.TransferDraft || o.Status == constants.TransferVoided {
			continue
		}
		out = append(out, *o)
	}
	return out, int64(len(out)), nil
}
func (m *mockTransferOrderRepo) ListDrafts(page, pageSize int, creatorID uint) ([]model.TransferOrder, int64, error) {
	var out []model.TransferOrder
	for _, o := range m.items {
		if o.CreatorID == creatorID && (o.Status == constants.TransferDraft || o.Status == constants.TransferVoided) {
			out = append(out, *o)
		}
	}
	return out, int64(len(out)), nil
}
func (m *mockTransferOrderRepo) Update(o *model.TransferOrder) error {
	cp := *o
	m.items[o.ID] = &cp
	return nil
}
func (m *mockTransferOrderRepo) UpdateTx(tx *gorm.DB, o *model.TransferOrder) error { return m.Update(o) }
func (m *mockTransferOrderRepo) TransitionStatusTx(tx *gorm.DB, id uint, from, to constants.TransferStatus) error {
	o, ok := m.items[id]
	if !ok || o.Status != from {
		return util.ErrConflict
	}
	o.Status = to
	return nil
}

func newTestTransferService() (TransferOrderService, StoreInventoryService, *mockTransferOrderRepo) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	invSvc, _ := newTestInventoryService()
	orderRepo := newMockTransferOrderRepo()
	storeRepo := &mockStoreRepo{ids: map[uint]bool{1: true, 2: true, 3: true}}
	// recSvc、db 草稿流程不使用，传 nil；Create 走 invSvc 内存校验。
	svc := NewTransferOrderService(orderRepo, storeRepo, &mockSKURepo{}, invSvc, nil, nil, logger)
	return svc, invSvc, orderRepo
}

func TestSaveDraft(t *testing.T) {
	svc, _, repo := newTestTransferService()
	// 库存不足也能保存：不预置库存，数量给大值。
	d, err := svc.SaveDraft(100, 1, 2, 50, 99999, "缺货也要先存着")
	if err != nil {
		t.Fatalf("save draft even with low stock: %v", err)
	}
	if d.Status != constants.TransferDraft || d.CreatorID != 100 {
		t.Fatalf("unexpected draft %+v", d)
	}
	// 普通列表不出现草稿。
	list, total, err := svc.List(1, 10, 0, "")
	if err != nil || total != 0 || len(list) != 0 {
		t.Fatalf("draft must not enter confirm queue, total=%d err=%v", total, err)
	}
	// 只有创建人自己的草稿列表能看到。
	if _, n, err := svc.ListDrafts(1, 10, 100); err != nil || n != 1 {
		t.Fatalf("owner should see draft, n=%d err=%v", n, err)
	}
	if _, n, _ := svc.ListDrafts(1, 10, 200); n != 0 {
		t.Fatalf("other manager should not see draft, n=%d", n)
	}
	_ = repo
}

func TestSaveDraftValidation(t *testing.T) {
	svc, _, _ := newTestTransferService()
	tests := []struct {
		name                       string
		from, to, sku              uint
		qty                        int
	}{
		{"same store", 1, 1, 10, 1},
		{"zero qty", 1, 2, 10, 0},
		{"negative qty", 1, 2, 10, -3},
		{"missing sku", 1, 2, 0, 1},
		{"missing from store", 0, 2, 10, 1},
		{"nonexistent to store", 1, 99, 10, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.SaveDraft(100, tt.from, tt.to, tt.sku, tt.qty, "")
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !errors.Is(err, util.ErrValidation) && !errors.Is(err, util.ErrNotFound) {
				t.Fatalf("expected validation/notfound, got %v", err)
			}
		})
	}
}

func TestDraftOwnership(t *testing.T) {
	svc, _, _ := newTestTransferService()
	d, err := svc.SaveDraft(100, 1, 2, 50, 5, "")
	if err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	// 其他角色/其他人不能编辑、作废、提交。
	if _, err := svc.UpdateDraft(d.ID, 200, 1, 2, 50, 6, ""); !errors.Is(err, util.ErrForbidden) {
		t.Fatalf("update by other should be forbidden, got %v", err)
	}
	if _, err := svc.VoidDraft(d.ID, 200); !errors.Is(err, util.ErrForbidden) {
		t.Fatalf("void by other should be forbidden, got %v", err)
	}
	if _, err := svc.SubmitDraft(d.ID, 200); !errors.Is(err, util.ErrForbidden) {
		t.Fatalf("submit by other should be forbidden, got %v", err)
	}
	// 本人可编辑。
	if _, err := svc.UpdateDraft(d.ID, 100, 1, 3, 50, 8, "改数量"); err != nil {
		t.Fatalf("owner update: %v", err)
	}
}

func TestSubmitDraftStock(t *testing.T) {
	svc, invSvc, _ := newTestTransferService()
	// 门店1 / SKU10 库存 5。
	if _, err := invSvc.Ensure(1, 10, 5); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	// 草稿数量 10 > 库存 5：暂存成功，提交应失败且保留草稿。
	d, err := svc.SaveDraft(100, 1, 2, 10, 10, "")
	if err != nil {
		t.Fatalf("save draft: %v", err)
	}
	if _, err := svc.SubmitDraft(d.ID, 100); !errors.Is(err, util.ErrStockNotEnough) {
		t.Fatalf("submit with low stock should fail, got %v", err)
	}
	kept, _, _ := svc.ListDrafts(1, 10, 100)
	if len(kept) != 1 || kept[0].Status != constants.TransferDraft {
		t.Fatalf("draft should be kept after failed submit, got %+v", kept)
	}
	// 补库存至 10 后提交成功，转为待确认并离开草稿队列。
	if err := invSvc.AdjustQuantity(1, 10, 5); err != nil {
		t.Fatalf("restock: %v", err)
	}
	submitted, err := svc.SubmitDraft(d.ID, 100)
	if err != nil {
		t.Fatalf("submit after restock: %v", err)
	}
	if submitted.Status != constants.TransferPending {
		t.Fatalf("status=%s want pending", submitted.Status)
	}
	if _, n, _ := svc.ListDrafts(1, 10, 100); n != 0 {
		t.Fatalf("submitted draft must leave draft list, n=%d", n)
	}
}

func TestVoidDraft(t *testing.T) {
	svc, _, _ := newTestTransferService()
	d, _ := svc.SaveDraft(100, 1, 2, 10, 1, "")
	voided, err := svc.VoidDraft(d.ID, 100)
	if err != nil {
		t.Fatalf("void: %v", err)
	}
	if voided.Status != constants.TransferVoided {
		t.Fatalf("status=%s want voided", voided.Status)
	}
	// 已作废不能再次编辑/提交/作废。
	if _, err := svc.SubmitDraft(d.ID, 100); !errors.Is(err, util.ErrConflict) {
		t.Fatalf("submit voided should conflict, got %v", err)
	}
	// 作废记录不进入普通确认队列。
	if _, total, _ := svc.List(1, 10, 0, ""); total != 0 {
		t.Fatalf("voided draft must not enter confirm queue, total=%d", total)
	}
}
