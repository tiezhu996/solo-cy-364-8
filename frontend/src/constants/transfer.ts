// TransferStatus 调拨单状态枚举（与 backend/internal/constants/transfer.go 对应）
export const TransferStatus = {
  DRAFT: 'draft',
  PENDING: 'pending',
  CONFIRMED: 'confirmed',
  SHIPPED: 'shipped',
  RECEIVED: 'received',
  CANCELLED: 'cancelled',
  VOIDED: 'voided'
} as const

export type TransferStatusValue = (typeof TransferStatus)[keyof typeof TransferStatus]

// 确认队列筛选项：草稿与草稿作废不进入调拨主列表，故不在此出现。
export const TRANSFER_STATUS_OPTIONS: { label: string; value: TransferStatusValue }[] = [
  { label: '待确认', value: TransferStatus.PENDING },
  { label: '已确认', value: TransferStatus.CONFIRMED },
  { label: '已发货', value: TransferStatus.SHIPPED },
  { label: '已收货', value: TransferStatus.RECEIVED },
  { label: '已取消', value: TransferStatus.CANCELLED }
]

export const TRANSFER_STATUS_TEXT: Record<TransferStatusValue, string> = {
  [TransferStatus.DRAFT]: '草稿',
  [TransferStatus.PENDING]: '待确认',
  [TransferStatus.CONFIRMED]: '已确认',
  [TransferStatus.SHIPPED]: '已发货',
  [TransferStatus.RECEIVED]: '已收货',
  [TransferStatus.CANCELLED]: '已取消',
  [TransferStatus.VOIDED]: '已作废'
}

// 状态机：允许的流转，与后端 CanTransfer 对应。
// 草稿只能经"提交"进入待确认；草稿作废落到 voided，独立于取消流。
export const TRANSFER_STATUS_FLOW: Record<TransferStatusValue, TransferStatusValue[]> = {
  [TransferStatus.DRAFT]: [TransferStatus.PENDING],
  [TransferStatus.PENDING]: [TransferStatus.CONFIRMED, TransferStatus.CANCELLED],
  [TransferStatus.CONFIRMED]: [TransferStatus.SHIPPED, TransferStatus.CANCELLED],
  [TransferStatus.SHIPPED]: [TransferStatus.RECEIVED, TransferStatus.CANCELLED],
  [TransferStatus.RECEIVED]: [],
  [TransferStatus.CANCELLED]: [],
  [TransferStatus.VOIDED]: []
}

export function canTransfer(from: TransferStatusValue, to: TransferStatusValue): boolean {
  return TRANSFER_STATUS_FLOW[from]?.includes(to) ?? false
}
