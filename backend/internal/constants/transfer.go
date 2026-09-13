package constants

// TransferStatus 调拨单状态枚举，前后端共享定义（frontend/src/constants/transfer.ts 对应实现）。
type TransferStatus string

const (
	TransferDraft     TransferStatus = "draft"
	TransferPending   TransferStatus = "pending"
	TransferConfirmed TransferStatus = "confirmed"
	TransferShipped   TransferStatus = "shipped"
	TransferReceived  TransferStatus = "received"
	TransferCancelled TransferStatus = "cancelled"
	TransferVoided    TransferStatus = "voided"
)

func (s TransferStatus) Valid() bool {
	switch s {
	case TransferDraft, TransferPending, TransferConfirmed, TransferShipped, TransferReceived, TransferCancelled, TransferVoided:
		return true
	}
	return false
}

// TransferStatusFlow 定义调拨单允许的合法状态流转，状态机在 service、前端按钮、日志模板、错误码、formatters 中多处耦合。
// 草稿（draft）仅能通过"提交"进入待确认（pending）；草稿作废走独立的作废动作落到 voided，不并入取消流。
var TransferStatusFlow = map[TransferStatus][]TransferStatus{
	TransferDraft:     {TransferPending},
	TransferPending:   {TransferConfirmed, TransferCancelled},
	TransferConfirmed: {TransferShipped, TransferCancelled},
	TransferShipped:   {TransferReceived, TransferCancelled},
	TransferReceived:  {},
	TransferCancelled: {},
	TransferVoided:    {},
}

func CanTransfer(from, to TransferStatus) bool {
	for _, next := range TransferStatusFlow[from] {
		if next == to {
			return true
		}
	}
	return false
}
