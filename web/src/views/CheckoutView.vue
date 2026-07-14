<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Check, Plus, CircleCheck as ShieldCheck } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { orderApi, userApi } from '../api/index.js'
import { useCheckoutStore } from '../stores/checkout.js'
import { cityText, dateTime, maskCertificate, money, placeText } from '../utils/format.js'
import { posterFor } from '../utils/posters.js'
import EmptyState from '../components/EmptyState.vue'

const router = useRouter()
const checkout = useCheckoutStore()
const users = ref([])
const selectedUsers = ref([])
const loading = ref(true)
const submitting = ref(false)
const requestId = crypto.randomUUID()
const required = computed(() => checkout.ticketCount)
const canSubmit = computed(() => checkout.draft && selectedUsers.value.length === required.value)

async function load() { if (!checkout.draft) { loading.value = false; return } loading.value = true; try { const data = await userApi.ticketUsers(); users.value = data.ticket_users || [] } finally { loading.value = false } }
function toggleUser(id) { const index = selectedUsers.value.indexOf(id); if (index >= 0) selectedUsers.value.splice(index,1); else if (selectedUsers.value.length < required.value) selectedUsers.value.push(id) }
async function submit() {
  if (!canSubmit.value) return
  submitting.value = true
  try {
    const data = await orderApi.create({ program_id: checkout.draft.program.id, ticket_category_id: checkout.draft.category.id, seat_ids: checkout.draft.seatIds, ticket_user_ids: selectedUsers.value }, requestId)
    const orderNumber = data.order_number
    checkout.clear()
    ElMessage.success('订单创建成功，请及时支付')
    router.replace(`/orders/${orderNumber}`)
  } finally { submitting.value = false }
}
onMounted(load)
</script>

<template>
  <section class="container page checkout-page">
    <template v-if="checkout.draft">
      <div><p class="eyebrow">Checkout</p><h1 class="page-title">确认订单</h1><p class="page-lead">核对演出和实名信息，提交后系统将锁定库存。</p></div>
      <div class="checkout-layout">
        <div class="checkout-main">
          <section class="surface event-summary"><img :src="posterFor(checkout.draft.program.id)" :alt="`${checkout.draft.program.title} 海报`" /><div><h2>{{ checkout.draft.program.title }}</h2><p>{{ dateTime(checkout.draft.program.show_time) }}</p><p>{{ cityText(checkout.draft.program.city) }} · {{ placeText(checkout.draft.program.place) }}</p><div><span>{{ checkout.draft.category.name }}</span><span v-if="checkout.draft.seats.length">{{ checkout.draft.seats.map(seat => `${seat.row_code}排${seat.col_code}座`).join('、') }}</span><span v-else>{{ required }} 张</span></div></div></section>
          <section class="people-section"><div class="section-heading"><div><h2>选择购票人</h2><p>请选择 {{ required }} 位，当前已选 {{ selectedUsers.length }} 位。</p></div><el-button :icon="Plus" @click="router.push('/ticket-users')">管理购票人</el-button></div>
            <div v-if="loading" class="surface loading-users"><el-skeleton :rows="4" animated /></div>
            <div v-else-if="users.length" class="people-list"><button v-for="user in users" :key="user.id" :class="['person-row', { selected: selectedUsers.includes(user.id) }]" @click="toggleUser(user.id)"><span class="check"><el-icon v-if="selectedUsers.includes(user.id)"><Check /></el-icon></span><span><strong>{{ user.name }}</strong><small>{{ maskCertificate(user.certificate_no) }}</small></span><em>实名</em></button></div>
            <div v-else class="surface"><EmptyState title="请先添加购票人" description="每张门票需要绑定一位实名购票人。"><el-button type="primary" :icon="Plus" @click="router.push('/ticket-users')">添加购票人</el-button></EmptyState></div>
          </section>
        </div>
        <aside class="order-summary surface"><h2>费用明细</h2><div><span>{{ checkout.draft.category.name }} × {{ required }}</span><strong>{{ money(checkout.amountCent) }}</strong></div><div><span>服务费</span><strong>{{ money(0) }}</strong></div><div class="total"><span>应付金额</span><strong>{{ money(checkout.amountCent) }}</strong></div><el-button type="primary" size="large" :loading="submitting" :disabled="!canSubmit" @click="submit">提交订单</el-button><p><el-icon><ShieldCheck /></el-icon>金额由服务端根据票档重新计算</p></aside>
      </div>
    </template>
    <div v-else class="surface"><EmptyState title="没有待确认的演出" description="请先选择节目、票档与座位。"><el-button type="primary" @click="router.push('/')">去发现演出</el-button></EmptyState></div>
  </section>
</template>

<style scoped>
.checkout-layout { display: grid; grid-template-columns: minmax(0,1fr) 330px; gap: 26px; align-items: start; margin-top: 28px; }.checkout-main { display: grid; gap: 34px; }.event-summary { display: grid; grid-template-columns: 118px 1fr; gap: 20px; padding: 20px; }.event-summary img { width: 118px; aspect-ratio: 3/4; object-fit: cover; border-radius: 6px; }.event-summary h2 { margin: 3px 0 14px; font-size: 20px; }.event-summary p { margin: 7px 0; color: var(--muted); font-size: 13px; }.event-summary div div { display: flex; flex-wrap: wrap; gap: 7px; margin-top: 14px; }.event-summary div div span { padding: 5px 8px; border-radius: 4px; background: var(--soft); font-size: 12px; }.people-section .section-heading { margin-bottom: 14px; }.people-list { display: grid; gap: 9px; }.person-row { width: 100%; min-height: 72px; display: grid; grid-template-columns: 24px 1fr auto; align-items: center; gap: 12px; padding: 13px 16px; border: 1px solid var(--line); border-radius: 7px; background: #fff; cursor: pointer; text-align: left; }.person-row.selected { border-color: var(--accent); background: #fff6f7; }.check { width: 20px; height: 20px; display: grid; place-items: center; border: 1px solid #b8bdc5; border-radius: 4px; color: #fff; }.person-row.selected .check { border-color: var(--accent); background: var(--accent); }.person-row > span:nth-child(2) { display: grid; gap: 5px; }.person-row small { color: var(--muted); }.person-row em { color: var(--teal); font-size: 12px; font-style: normal; }.order-summary { position: sticky; top: 92px; display: grid; gap: 16px; padding: 22px; box-shadow: var(--shadow); }.order-summary h2 { margin: 0 0 5px; font-size: 19px; }.order-summary > div { display: flex; justify-content: space-between; gap: 15px; color: var(--muted); font-size: 13px; }.order-summary > div strong { color: var(--ink); }.order-summary .total { align-items: baseline; margin-top: 5px; padding-top: 17px; border-top: 1px solid var(--line); }.order-summary .total strong { color: var(--accent); font-size: 24px; }.order-summary p { display: flex; align-items: center; justify-content: center; gap: 5px; margin: 0; color: var(--muted); font-size: 11px; }.order-summary p .el-icon { color: var(--teal); }.loading-users { padding: 20px; }
@media (max-width: 820px) { .checkout-layout { grid-template-columns: 1fr; }.order-summary { position: static; grid-row: 1; } }
@media (max-width: 560px) { .event-summary { grid-template-columns: 86px 1fr; gap: 14px; padding: 14px; }.event-summary img { width: 86px; }.event-summary h2 { font-size: 16px; }.people-section .section-heading { align-items: stretch; flex-direction: column; }.person-row { padding-inline: 12px; } }
</style>
