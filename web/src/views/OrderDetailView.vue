<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Calendar as CalendarDays, Location as MapPin, CreditCard, CircleCheck, Clock as Clock3 } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { orderApi, paymentApi, programApi } from '../api/index.js'
import { cityText, dateTime, money, placeText } from '../utils/format.js'
import { posterFor } from '../utils/posters.js'
import EmptyState from '../components/EmptyState.vue'
import OrderStatus from '../components/OrderStatus.vue'

const route = useRoute()
const router = useRouter()
const loading = ref(true)
const processing = ref(false)
const order = ref(null)
const program = ref(null)
const category = computed(() => program.value?.ticket_categories?.find((item) => item.id === order.value?.ticket_category_id))

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
async function load(retry = true) {
  loading.value = true
  try {
    let lastError
    for (let attempt = 0; attempt < (retry ? 8 : 1); attempt += 1) {
      try { order.value = await orderApi.detail(route.params.orderNumber, retry); lastError = null; break } catch (error) { lastError = error; await sleep(500) }
    }
    if (lastError) throw lastError
    program.value = await programApi.detail(order.value.program_id)
  } finally { loading.value = false }
}

async function cancelOrder() {
  await ElMessageBox.confirm('取消后已锁定的库存将被释放，确定继续吗？', '取消订单', { confirmButtonText: '确认取消', cancelButtonText: '暂不取消', type: 'warning' })
  processing.value = true
  try { order.value = await orderApi.cancel(order.value.order_number); ElMessage.success('订单已取消') } finally { processing.value = false }
}

async function payOrder() {
  await ElMessageBox.confirm(`确认使用本地模拟支付完成 ${money(order.value.amount_cent)} 的订单吗？`, '模拟支付', { confirmButtonText: '确认支付', cancelButtonText: '稍后支付', type: 'info' })
  processing.value = true
  try {
    await paymentApi.create({ order_number: order.value.order_number, amount_cent: order.value.amount_cent, channel: 'mock' })
    await paymentApi.callback({ order_number: order.value.order_number, amount_cent: order.value.amount_cent, channel: 'mock', paid: true })
    ElMessage.success('支付成功')
    await load(false)
  } finally { processing.value = false }
}

onMounted(() => load(true))
</script>

<template>
  <section class="container page detail-order-page">
    <button class="back-link" @click="router.push('/orders')"><el-icon><ArrowLeft /></el-icon>返回订单列表</button>
    <div v-if="loading" class="surface loading-card"><el-skeleton :rows="9" animated /></div>
    <template v-else-if="order">
      <div class="order-heading surface"><div class="status-icon" :class="order.status"><el-icon><Clock3 v-if="order.status === 'NO_PAY'" /><CircleCheck v-else /></el-icon></div><div><div class="status-title"><h1>{{ order.status === 'NO_PAY' ? '等待支付' : order.status === 'PAY' ? '订单已完成' : '订单已关闭' }}</h1><OrderStatus :status="order.status" /></div><p>{{ order.status === 'NO_PAY' ? '座位已锁定，请尽快完成支付。' : '订单状态已更新，可在此查看完整信息。' }}</p><small>订单号 {{ order.order_number }}</small></div><div v-if="order.status === 'NO_PAY'" class="heading-actions"><el-button :loading="processing" @click="cancelOrder">取消订单</el-button><el-button type="primary" :icon="CreditCard" :loading="processing" @click="payOrder">立即支付</el-button></div></div>
      <div class="order-detail-layout">
        <div class="surface detail-content">
          <section class="event-row"><img :src="posterFor(order.program_id)" :alt="`${program?.title || '演出'} 海报`" /><div><p class="eyebrow">演出信息</p><h2>{{ program?.title || `节目 ${order.program_id}` }}</h2><p><el-icon><CalendarDays /></el-icon>{{ dateTime(program?.show_time) }}</p><p><el-icon><MapPin /></el-icon>{{ cityText(program?.city) }} · {{ placeText(program?.place) }}</p></div></section>
          <div class="divider"></div>
          <section class="detail-block"><h3>票品信息</h3><dl><div><dt>票档</dt><dd>{{ category?.name || order.ticket_category_id }}</dd></div><div><dt>座位</dt><dd>{{ order.seat_ids?.length ? order.seat_ids.join('、') : `${order.ticket_user_ids?.length || 1} 张不选座票` }}</dd></div><div><dt>购票人数</dt><dd>{{ order.ticket_user_ids?.length || order.seat_ids?.length || 1 }} 人</dd></div></dl></section>
          <div class="divider"></div>
          <section class="detail-block"><h3>订单信息</h3><dl><div><dt>创建时间</dt><dd>{{ dateTime(order.created_at) }}</dd></div><div><dt>订单编号</dt><dd class="order-number">{{ order.order_number }}</dd></div><div><dt>支付渠道</dt><dd>{{ order.status === 'PAY' ? '本地模拟支付' : '尚未支付' }}</dd></div></dl></section>
        </div>
        <aside class="surface amount-panel"><h2>金额明细</h2><div><span>门票金额</span><strong>{{ money(order.amount_cent) }}</strong></div><div><span>服务费</span><strong>{{ money(0) }}</strong></div><div class="amount-total"><span>订单总额</span><strong>{{ money(order.amount_cent) }}</strong></div><p>订单价格由 TicketHub 服务端权威计算。</p></aside>
      </div>
    </template>
    <div v-else class="surface"><EmptyState title="暂时找不到这个订单" description="异步下单可能需要几秒钟，请稍后刷新。"><el-button type="primary" @click="load(true)">重新加载</el-button></EmptyState></div>
  </section>
</template>

<style scoped>
.back-link { display: flex; align-items: center; gap: 6px; margin-bottom: 20px; padding: 0; border: 0; background: transparent; color: var(--muted); cursor: pointer; }.loading-card { padding: 35px; }.order-heading { display: grid; grid-template-columns: 52px minmax(0,1fr) auto; align-items: center; gap: 18px; padding: 24px; }.status-icon { width: 52px; height: 52px; display: grid; place-items: center; border-radius: 50%; background: #fff3dc; color: #b66c00; font-size: 24px; }.status-icon.PAY { background: #e5f5ef; color: #087f5b; }.status-icon.CANCEL,.status-icon.REFUND { background: var(--soft); color: #747a83; }.status-title { display: flex; align-items: center; gap: 10px; }.status-title h1 { margin: 0; font-size: 22px; }.order-heading p { margin: 7px 0; color: var(--muted); }.order-heading small { color: #8b9098; }.heading-actions { display: flex; }.order-detail-layout { display: grid; grid-template-columns: minmax(0,1fr) 320px; gap: 24px; align-items: start; margin-top: 18px; }.detail-content { padding: 24px; }.event-row { display: grid; grid-template-columns: 105px 1fr; gap: 20px; }.event-row img { width: 105px; aspect-ratio: 3/4; object-fit: cover; border-radius: 6px; }.event-row h2 { margin: 0 0 17px; font-size: 20px; }.event-row p:not(.eyebrow) { display: flex; align-items: center; gap: 7px; margin: 9px 0; color: var(--muted); font-size: 13px; }.detail-content .divider { margin: 25px 0; }.detail-block h3 { margin: 0 0 16px; font-size: 16px; }.detail-block dl { display: grid; gap: 12px; margin: 0; }.detail-block dl div { display: grid; grid-template-columns: 110px 1fr; gap: 14px; font-size: 13px; }.detail-block dt { color: var(--muted); }.detail-block dd { margin: 0; word-break: break-all; }.amount-panel { display: grid; gap: 16px; padding: 22px; }.amount-panel h2 { margin: 0 0 5px; font-size: 19px; }.amount-panel > div { display: flex; justify-content: space-between; color: var(--muted); font-size: 13px; }.amount-panel > div strong { color: var(--ink); }.amount-panel .amount-total { align-items: baseline; margin-top: 4px; padding-top: 17px; border-top: 1px solid var(--line); }.amount-panel .amount-total strong { color: var(--accent); font-size: 23px; }.amount-panel p { margin: 0; color: var(--muted); font-size: 11px; line-height: 1.6; }
@media (max-width: 780px) { .order-heading { grid-template-columns: 46px 1fr; padding: 18px; }.status-icon { width: 46px; height: 46px; }.heading-actions { grid-column: 1/-1; display: grid; grid-template-columns: 1fr 1fr; }.order-detail-layout { grid-template-columns: 1fr; }.amount-panel { grid-row: 1; } }
@media (max-width: 520px) { .status-title { align-items: start; flex-direction: column; gap: 6px; }.status-title h1 { font-size: 19px; }.order-heading p { font-size: 13px; }.event-row { grid-template-columns: 78px 1fr; gap: 14px; }.event-row img { width: 78px; }.event-row h2 { font-size: 16px; }.detail-content { padding: 17px; }.detail-block dl div { grid-template-columns: 82px 1fr; } }
</style>
