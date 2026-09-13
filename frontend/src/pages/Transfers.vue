<template>
  <div>
    <el-card>
      <el-tabs v-model="activeTab" @tab-change="onTabChange">
        <!-- ===== 调拨单（确认队列，草稿不进入） ===== -->
        <el-tab-pane label="调拨单" name="orders">
          <div class="toolbar">
            <el-select v-model="statusFilter" placeholder="状态筛选" clearable style="width: 160px" @change="reloadOrders">
              <el-option v-for="s in TRANSFER_STATUS_OPTIONS" :key="s.value" :label="s.label" :value="s.value" />
            </el-select>
            <el-button type="primary" @click="reloadOrders">查询</el-button>
            <div class="spacer"></div>
            <el-button type="primary" @click="openCreate">发起调拨</el-button>
          </div>
          <el-table :data="list" v-loading="loading" border stripe>
            <el-table-column prop="id" label="ID" width="70" />
            <el-table-column label="调出门店" min-width="140">
              <template #default="{ row }">{{ row.from_store?.name || `#${row.from_store_id}` }}</template>
            </el-table-column>
            <el-table-column label="调入门店" min-width="140">
              <template #default="{ row }">{{ row.to_store?.name || `#${row.to_store_id}` }}</template>
            </el-table-column>
            <el-table-column label="商品" min-width="150">
              <template #default="{ row }">{{ row.sku?.name || `#${row.sku_id}` }}</template>
            </el-table-column>
            <el-table-column prop="quantity" label="数量" width="90" />
            <el-table-column prop="reason" label="原因" min-width="120" show-overflow-tooltip />
            <el-table-column label="状态" width="100">
              <template #default="{ row }"><TransferStatusBadge :status="row.status" /></template>
            </el-table-column>
            <el-table-column label="操作" width="240" fixed="right">
              <template #default="{ row }">
                <el-button v-if="canConfirm(row)" link type="primary" size="small" @click="doConfirm(row)">确认</el-button>
                <el-button v-if="canShip(row)" link type="warning" size="small" @click="doShip(row)">发货</el-button>
                <el-button v-if="canReceive(row)" link type="success" size="small" @click="doReceive(row)">收货</el-button>
                <el-button v-if="canCancel(row)" link type="danger" size="small" @click="doCancel(row)">取消</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-pagination class="mt" layout="total, prev, pager, next" :total="total" :page-size="pageSize" v-model:current-page="page" @current-change="loadOrders" />
        </el-tab-pane>

        <!-- ===== 我的草稿（仅店长本人可见） ===== -->
        <el-tab-pane v-if="isManager" label="我的草稿" name="drafts">
          <div class="toolbar">
            <span class="hint">草稿不进入确认队列，库存不足也可暂存；提交时才校验库存。仅本人可编辑/作废。</span>
            <div class="spacer"></div>
            <el-button type="primary" @click="openCreateDraft">新建草稿</el-button>
          </div>
          <el-table :data="drafts" v-loading="draftLoading" border stripe>
            <el-table-column prop="id" label="ID" width="70" />
            <el-table-column label="调出门店" min-width="140">
              <template #default="{ row }">{{ row.from_store?.name || `#${row.from_store_id}` }}</template>
            </el-table-column>
            <el-table-column label="调入门店" min-width="140">
              <template #default="{ row }">{{ row.to_store?.name || `#${row.to_store_id}` }}</template>
            </el-table-column>
            <el-table-column label="商品" min-width="150">
              <template #default="{ row }">{{ row.sku?.name || `#${row.sku_id}` }}</template>
            </el-table-column>
            <el-table-column prop="quantity" label="数量" width="90" />
            <el-table-column prop="reason" label="原因" min-width="120" show-overflow-tooltip />
            <el-table-column label="状态" width="100">
              <template #default="{ row }"><TransferStatusBadge :status="row.status" /></template>
            </el-table-column>
            <el-table-column label="操作" width="220" fixed="right">
              <template #default="{ row }">
                <template v-if="row.status === TransferStatus.DRAFT">
                  <el-button link type="primary" size="small" @click="openEditDraft(row)">编辑</el-button>
                  <el-button link type="success" size="small" :loading="submittingId === row.id" @click="doSubmitDraft(row)">提交</el-button>
                  <el-button link type="danger" size="small" @click="doVoidDraft(row)">作废</el-button>
                </template>
                <span v-else class="muted">—</span>
              </template>
            </el-table-column>
          </el-table>
          <el-pagination class="mt" layout="total, prev, pager, next" :total="draftTotal" :page-size="draftPageSize" v-model:current-page="draftPage" @current-change="loadDrafts" />
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <!-- 新建调拨 / 新建·编辑草稿 共用弹窗 -->
    <el-dialog v-model="formVisible" :title="dialogTitle" width="500px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="调出门店" required>
          <el-select v-model="form.from_store_id" style="width: 100%">
            <el-option v-for="s in stores" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="调入门店" required>
          <el-select v-model="form.to_store_id" style="width: 100%">
            <el-option v-for="s in stores" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="商品" required>
          <el-select v-model="form.sku_id" style="width: 100%">
            <el-option v-for="s in skus" :key="s.id" :label="`${s.name}（${s.code}）`" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="数量" required>
          <el-input-number v-model="form.quantity" :min="1" />
        </el-form-item>
        <el-form-item label="原因">
          <el-input v-model="form.reason" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="formVisible = false">取消</el-button>
        <el-button v-if="isManager" :loading="savingDraft" @click="onSaveDraft">
          {{ dialogMode === 'editDraft' ? '保存草稿' : '暂存草稿' }}
        </el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmitForm">
          {{ dialogMode === 'editDraft' ? '提交草稿' : '提交' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import TransferStatusBadge from '@/components/common/TransferStatusBadge.vue'
import {
  listTransfers, createTransfer, confirmTransfer, shipTransfer, receiveTransfer, cancelTransfer,
  listTransferDrafts, saveTransferDraft, updateTransferDraft, voidTransferDraft, submitTransferDraft
} from '@/api/transferOrder'
import { listAllStores } from '@/api/store'
import { listSkus } from '@/api/sku'
import { TRANSFER_STATUS_OPTIONS, TransferStatus, canTransfer, type TransferStatusValue } from '@/constants/transfer'
import { UserRole } from '@/constants/user'
import { useAuthStore } from '@/stores/authStore'
import type { Store, SKU, TransferOrder } from '@/types'

const auth = useAuthStore()
const isManager = computed(() => auth.role === UserRole.STORE_MANAGER)

// ---- 调拨单主列表 ----
const list = ref<TransferOrder[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
const statusFilter = ref<TransferStatusValue | ''>('')
const loading = ref(false)

// ---- 我的草稿 ----
const activeTab = ref<'orders' | 'drafts'>('orders')
const drafts = ref<TransferOrder[]>([])
const draftTotal = ref(0)
const draftPage = ref(1)
const draftPageSize = ref(10)
const draftLoading = ref(false)
const submittingId = ref(0)

const stores = ref<Store[]>([])
const skus = ref<SKU[]>([])
const saving = ref(false)
const submitting = ref(false)
const savingDraft = ref(false)
const formVisible = ref(false)
// create=直接发起调拨；createDraft=新建草稿；editDraft=编辑已有草稿
const dialogMode = ref<'create' | 'createDraft' | 'editDraft'>('create')
const editingDraftId = ref(0)
const form = reactive({ from_store_id: 0, to_store_id: 0, sku_id: 0, quantity: 1, reason: '' })

const dialogTitle = computed(() => {
  if (dialogMode.value === 'editDraft') return '编辑调拨草稿'
  if (dialogMode.value === 'createDraft') return '新建调拨草稿'
  return '发起调拨申请'
})

async function loadOrders() {
  loading.value = true
  try {
    const params: { page: number; page_size: number; status?: TransferStatusValue } = { page: page.value, page_size: pageSize.value }
    if (statusFilter.value) params.status = statusFilter.value
    const res = await listTransfers(params)
    list.value = res.list
    total.value = res.total
  } finally {
    loading.value = false
  }
}
function reloadOrders() {
  page.value = 1
  return loadOrders()
}

async function loadDrafts() {
  draftLoading.value = true
  try {
    const res = await listTransferDrafts({ page: draftPage.value, page_size: draftPageSize.value })
    drafts.value = res.list
    draftTotal.value = res.total
  } finally {
    draftLoading.value = false
  }
}
function onTabChange(name: string | number) {
  if (name === 'drafts' && draftTotal.value === 0 && drafts.value.length === 0) {
    draftPage.value = 1
    loadDrafts()
  }
}

function resetForm() {
  form.from_store_id = stores.value[0]?.id || 0
  form.to_store_id = stores.value[1]?.id || 0
  form.sku_id = skus.value[0]?.id || 0
  form.quantity = 1
  form.reason = ''
}

function openCreate() {
  resetForm()
  dialogMode.value = 'create'
  editingDraftId.value = 0
  formVisible.value = true
}
function openCreateDraft() {
  resetForm()
  dialogMode.value = 'createDraft'
  editingDraftId.value = 0
  formVisible.value = true
}
function openEditDraft(row: TransferOrder) {
  form.from_store_id = row.from_store_id
  form.to_store_id = row.to_store_id
  form.sku_id = row.sku_id
  form.quantity = row.quantity
  form.reason = row.reason || ''
  dialogMode.value = 'editDraft'
  editingDraftId.value = row.id
  formVisible.value = true
}

// 暂存/编辑保存：两门店不能相同、商品与数量有效（数量组件已限定 >=1），不校验库存。
async function onSaveDraft() {
  if (!validateForm()) return
  savingDraft.value = true
  try {
    if (dialogMode.value === 'editDraft') {
      await updateTransferDraft(editingDraftId.value, { ...form })
      ElMessage.success('草稿已保存')
    } else {
      await saveTransferDraft({ ...form })
      ElMessage.success('调拨草稿已暂存')
    }
    formVisible.value = false
    if (activeTab.value === 'drafts') await loadDrafts()
  } finally {
    savingDraft.value = false
  }
}

// 弹窗提交：直接发起调拨，或把草稿提交为待确认（提交时才校验库存，不足则保留草稿）。
async function onSubmitForm() {
  if (!validateForm()) return
  submitting.value = true
  try {
    if (dialogMode.value === 'editDraft') {
      // 先保存编辑内容，再提交转待确认；库存不足时后端保留草稿。
      await updateTransferDraft(editingDraftId.value, { ...form })
      await submitTransferDraft(editingDraftId.value)
      ElMessage.success('草稿已提交，转为待确认')
      formVisible.value = false
      await loadDrafts()
    } else {
      await createTransfer({ ...form })
      ElMessage.success('调拨申请已提交')
      formVisible.value = false
      await reloadOrders()
    }
  } finally {
    submitting.value = false
  }
}

function validateForm(): boolean {
  if (!form.from_store_id || !form.to_store_id) {
    ElMessage.warning('请选择调出/调入门店')
    return false
  }
  if (form.from_store_id === form.to_store_id) {
    ElMessage.warning('调出门店与调入门店不能相同')
    return false
  }
  if (!form.sku_id) {
    ElMessage.warning('请选择商品')
    return false
  }
  if (!form.quantity || form.quantity <= 0) {
    ElMessage.warning('数量必须大于 0')
    return false
  }
  return true
}

function canConfirm(row: TransferOrder) {
  return (auth.role === UserRole.ADMIN || auth.role === UserRole.HQ) && canTransfer(row.status, TransferStatus.CONFIRMED)
}
function canShip(row: TransferOrder) {
  return canTransfer(row.status, TransferStatus.SHIPPED)
}
function canReceive(row: TransferOrder) {
  return canTransfer(row.status, TransferStatus.RECEIVED)
}
function canCancel(row: TransferOrder) {
  return canTransfer(row.status, TransferStatus.CANCELLED)
}

async function doConfirm(row: TransferOrder) { await confirmTransfer(row.id); ElMessage.success('已确认'); await loadOrders() }
async function doShip(row: TransferOrder) { await shipTransfer(row.id); ElMessage.success('已发货'); await loadOrders() }
async function doReceive(row: TransferOrder) { await receiveTransfer(row.id); ElMessage.success('已收货'); await loadOrders() }
async function doCancel(row: TransferOrder) { await cancelTransfer(row.id); ElMessage.success('已取消'); await loadOrders() }

async function doSubmitDraft(row: TransferOrder) {
  submittingId.value = row.id
  try {
    await submitTransferDraft(row.id)
    ElMessage.success('草稿已提交，转为待确认')
    await loadDrafts()
  } finally {
    submittingId.value = 0
  }
}
async function doVoidDraft(row: TransferOrder) {
  try {
    await ElMessageBox.confirm(`确定作废草稿 #${row.id} 吗？作废后不可恢复。`, '作废草稿', { type: 'warning' })
  } catch {
    return
  }
  await voidTransferDraft(row.id)
  ElMessage.success('草稿已作废')
  await loadDrafts()
}

onMounted(async () => {
  stores.value = await listAllStores()
  const res = await listSkus({ page: 1, page_size: 200 })
  skus.value = res.list
  await loadOrders()
})
</script>

<style scoped>
.toolbar { display: flex; gap: 10px; margin-bottom: 14px; align-items: center; }
.spacer { flex: 1; }
.mt { margin-top: 14px; }
.hint { color: var(--el-text-color-secondary); font-size: 13px; }
.muted { color: var(--el-text-color-placeholder); }
</style>
