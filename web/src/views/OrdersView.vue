<script setup>
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Calendar as CalendarDays, Right as ChevronRight, Refresh as RefreshCw } from '@element-plus/icons-vue'
import { orderApi, programApi } from '../api/index.js'
import { dateTime, money } from '../utils/format.js'
import { posterFor } from '../utils/posters.js'
import EmptyState from '../components/EmptyState.vue'
import OrderStatus from '../components/OrderStatus.vue'

const router = useRouter()
const orders = ref([])
const programs = ref({})
const loading = ref(true)
const loadingMore = ref(false)
const nextCursor = ref('')
const filter = ref('ALL')
const filters = [{ value: 'ALL', label: '全部' }, { value: 'NO_PAY', label: '待支付' }, { value: 'PAY', label: '已支付' }, { value: 'CANCEL', label: '已取消' }, { value: 'REFUND', label: '已退款' }]

async function load(reset = true) {
  if (reset) loading.value = true
  else loadingMore.value = true
  try {
    const data = await orderApi.list({ cursor: reset ? undefined : nextCursor.value, status: filter.value === 'ALL' ? undefined : filter.value })
    const incoming = data.orders || []
    orders.value = reset ? incoming : [...orders.value, ...incoming]
    nextCursor.value = data.next_cursor || ''
    const ids = [...new Set(incoming.map((item) => item.program_id).filter((id) => id && !programs.value[id]))]
    const details = await Promise.all(ids.map(async (id) => { try { return await programApi.detail(id) } catch { return { id, title: `节目 ${id}` } } }))
    programs.value = { ...programs.value, ...Object.fromEntries(details.map((item) => [item.id, item])) }
  } finally {
    loading.value = false
    loadingMore.value = false
  }
}

onMounted(load)
watch(filter, () => load(true))
</script>

<template>
  <section class="container page">
    <div class="section-heading"><div><h1 class="page-title">我的订单</h1><p class="page-lead">查看支付状态、演出信息和历史订单。</p></div><el-button :icon="RefreshCw" :loading="loading" @click="load">刷新</el-button></div>
    <div class="order-tabs"><button v-for="item in filters" :key="item.value" :class="{ active: filter === item.value }" @click="filter = item.value">{{ item.label }}</button></div>
    <div v-if="loading" class="order-list"><div v-for="i in 3" :key="i" class="surface order-skeleton"><el-skeleton :rows="3" animated /></div></div>
    <div v-else-if="orders.length" class="order-list"><article v-for="order in orders" :key="order.order_number" class="order-row surface" @click="router.push(`/orders/${order.order_number}`)"><img :src="posterFor(order.program_id)" :alt="`${programs[order.program_id]?.title || '演出'} 海报`" /><div class="order-main"><div class="order-title"><h2>{{ programs[order.program_id]?.title || `节目 ${order.program_id}` }}</h2><OrderStatus :status="order.status" /></div><p><el-icon><CalendarDays /></el-icon>{{ dateTime(programs[order.program_id]?.show_time || order.created_at) }}</p><small>订单号 {{ order.order_number }}</small></div><div class="order-price"><strong>{{ money(order.amount_cent) }}</strong><span>查看详情 <el-icon><ChevronRight /></el-icon></span></div></article><el-button v-if="nextCursor" :loading="loadingMore" @click="load(false)">加载更多</el-button></div>
    <div v-else class="surface"><EmptyState :title="filter === 'ALL' ? '还没有订单' : '没有该状态的订单'" description="去看看最近正在热售的现场。"><el-button type="primary" @click="router.push('/')">发现演出</el-button></EmptyState></div>
  </section>
</template>

<style scoped>
.order-tabs { display: flex; gap: 5px; margin-bottom: 18px; padding-bottom: 12px; border-bottom: 1px solid var(--line); overflow-x: auto; }.order-tabs button { flex: 0 0 auto; display: flex; align-items: center; gap: 6px; padding: 8px 12px; border: 0; border-radius: 6px; background: transparent; color: var(--muted); cursor: pointer; }.order-tabs button.active { background: #fff0f2; color: var(--accent); font-weight: 700; }.order-tabs span { min-width: 20px; padding: 1px 5px; border-radius: 9px; background: var(--soft); color: #777c85; font-size: 11px; }.order-list { display: grid; gap: 12px; }.order-row { display: grid; grid-template-columns: 90px minmax(0,1fr) 150px; gap: 18px; align-items: center; padding: 15px; cursor: pointer; transition: border-color .2s, box-shadow .2s; }.order-row:hover { border-color: #c9cdd3; box-shadow: 0 8px 24px rgba(23,25,28,.06); }.order-row img { width: 90px; height: 116px; object-fit: cover; border-radius: 6px; }.order-title { display: flex; align-items: center; gap: 12px; }.order-main h2 { margin: 0; font-size: 17px; }.order-main p { display: flex; align-items: center; gap: 6px; margin: 12px 0; color: var(--muted); font-size: 13px; }.order-main small { color: #8b9098; }.order-price { display: grid; justify-items: end; gap: 24px; }.order-price strong { font-size: 19px; }.order-price span { display: flex; align-items: center; color: var(--blue); font-size: 13px; }.order-skeleton { padding: 22px; }
@media (max-width: 650px) { .section-heading { align-items: start; }.order-row { grid-template-columns: 72px minmax(0,1fr); gap: 13px; padding: 12px; }.order-row img { width: 72px; height: 96px; }.order-title { align-items: start; flex-direction: column; gap: 6px; }.order-main h2 { font-size: 15px; }.order-main p { margin: 9px 0; font-size: 12px; }.order-main small { display: block; max-width: 100%; overflow: hidden; text-overflow: ellipsis; }.order-price { grid-column: 1/-1; display: flex; align-items: center; justify-content: space-between; padding-top: 10px; border-top: 1px solid var(--line); } }
</style>
