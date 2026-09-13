import request from '@/utils/request'
import type { PageResult, TransferOrder } from '@/types'
import type { TransferStatusValue } from '@/constants/transfer'

export interface TransferPayload {
  from_store_id: number
  to_store_id: number
  sku_id: number
  quantity: number
  reason?: string
}

export function listTransfers(params: { page?: number; page_size?: number; store_id?: number; status?: TransferStatusValue } = {}): Promise<PageResult<TransferOrder>> {
  return request.get('/transfers', { params })
}

export function createTransfer(data: TransferPayload): Promise<TransferOrder> {
  return request.post('/transfers', data)
}

export function confirmTransfer(id: number): Promise<TransferOrder> {
  return request.put(`/transfers/${id}/confirm`)
}

export function shipTransfer(id: number): Promise<TransferOrder> {
  return request.put(`/transfers/${id}/ship`)
}

export function receiveTransfer(id: number): Promise<TransferOrder> {
  return request.put(`/transfers/${id}/receive`)
}

export function cancelTransfer(id: number): Promise<TransferOrder> {
  return request.put(`/transfers/${id}/cancel`)
}

// ===== 草稿环节（仅店长本人可操作，库存不足也能暂存） =====

export function listTransferDrafts(params: { page?: number; page_size?: number } = {}): Promise<PageResult<TransferOrder>> {
  return request.get('/transfer-drafts', { params })
}

export function saveTransferDraft(data: TransferPayload): Promise<TransferOrder> {
  return request.post('/transfer-drafts', data)
}

export function updateTransferDraft(id: number, data: TransferPayload): Promise<TransferOrder> {
  return request.put(`/transfer-drafts/${id}`, data)
}

export function voidTransferDraft(id: number): Promise<TransferOrder> {
  return request.put(`/transfer-drafts/${id}/void`)
}

export function submitTransferDraft(id: number): Promise<TransferOrder> {
  return request.put(`/transfer-drafts/${id}/submit`)
}
