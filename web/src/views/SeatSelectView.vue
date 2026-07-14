<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Ticket as Armchair, InfoFilled as Info, Minus, Plus } from '@element-plus/icons-vue'
import { programApi } from '../api/index.js'
import { useCheckoutStore } from '../stores/checkout.js'
import { money } from '../utils/format.js'
import EmptyState from '../components/EmptyState.vue'

const route = useRoute()
const router = useRouter()
const checkout = useCheckoutStore()
const loading = ref(true)
const program = ref(null)
const selected = ref([])
const quantity = ref(1)
const categoryId = computed(() => String(route.query.category || ''))
const category = computed(() => program.value?.ticket_categories?.find((item) => item.id === categoryId.value))
const seats = computed(() => program.value?.seats || [])
const availableSeats = computed(() => seats.value.filter((item) => item.status === 'no_sold'))
const isGeneralAdmission = computed(() => !loading.value && seats.value.length === 0)
const count = computed(() => isGeneralAdmission.value ? quantity.value : selected.value.length)
const total = computed(() => count.value * (category.value?.price_cent || 0))

async function load() {
  loading.value = true
  try { program.value = await programApi.detail(route.params.id, categoryId.value) } finally { loading.value = false }
}

function toggle(seat) {
  if (seat.status !== 'no_sold') return
  const index = selected.value.indexOf(seat.id)
  if (index >= 0) selected.value.splice(index, 1)
  else if (selected.value.length < 6) selected.value.push(seat.id)
}

function continueCheckout() {
  checkout.setDraft({
    program: program.value,
    category: category.value,
    seatIds: [...selected.value],
    seats: seats.value.filter((seat) => selected.value.includes(seat.id)),
    quantity: count.value,
  })
  router.push('/checkout')
}

onMounted(load)
</script>

<template>
  <section class="container page seat-page">
    <button class="back-link" @click="router.back()"><el-icon><ArrowLeft /></el-icon>返回节目详情</button>
    <div v-if="loading" class="surface seat-loading"><el-skeleton :rows="9" animated /></div>
    <template v-else-if="program && category">
      <div class="seat-heading"><div><p class="eyebrow">{{ program.title }}</p><h1 class="page-title">{{ isGeneralAdmission ? '选择购票数量' : '选择你的座位' }}</h1><p class="page-lead">{{ category.name }} · {{ money(category.price_cent) }} / 张，单次最多选择 6 张。</p></div><div class="legend" v-if="!isGeneralAdmission"><span><i class="available"></i>可选</span><span><i class="selected"></i>已选</span><span><i class="unavailable"></i>不可选</span></div></div>

      <div class="seat-layout">
        <div class="seat-map surface">
          <template v-if="isGeneralAdmission">
            <div class="general-admission"><span class="general-icon"><el-icon><Armchair /></el-icon></span><h2>本票档无需选座</h2><p>入场后按主办方安排就座或站席，请选择购票数量。</p><el-input-number v-model="quantity" :min="1" :max="Math.min(6, category.remain)" size="large" :controls="false" aria-label="购票数量" /><div class="quantity-controls"><el-button circle :icon="Minus" :disabled="quantity <= 1" @click="quantity--" /><strong>{{ quantity }} 张</strong><el-button circle :icon="Plus" :disabled="quantity >= Math.min(6, category.remain)" @click="quantity++" /></div></div>
          </template>
          <template v-else>
            <div class="stage"><span>舞台方向</span></div>
            <div class="seat-grid" :style="{ '--columns': Math.max(4, ...seats.map((seat) => Number(seat.col_code) || 1)) }">
              <button v-for="seat in seats" :key="seat.id" :title="`${seat.row_code}排${seat.col_code}座`" :disabled="seat.status !== 'no_sold'" :class="['seat', { selected: selected.includes(seat.id), unavailable: seat.status !== 'no_sold' }]" :style="{ gridColumn: Number(seat.col_code) || 'auto' }" @click="toggle(seat)"><span>{{ seat.row_code }}{{ seat.col_code }}</span></button>
            </div>
            <div v-if="availableSeats.length" class="seat-tip"><el-icon><Info /></el-icon>座位锁定以提交订单时的实时结果为准。</div>
            <EmptyState v-else title="该票档暂时没有可选座位" description="返回详情页选择其他票档。"><el-button @click="router.back()">重新选择票档</el-button></EmptyState>
          </template>
        </div>

        <aside class="seat-summary surface">
          <h2>选座明细</h2><div class="summary-line"><span>票档</span><strong>{{ category.name }}</strong></div><div class="summary-line"><span>数量</span><strong>{{ count }} 张</strong></div><div v-if="selected.length" class="selected-list"><span v-for="seat in seats.filter((item) => selected.includes(item.id))" :key="seat.id">{{ seat.row_code }}排 {{ seat.col_code }}座</span></div><div class="summary-total"><span>合计</span><strong>{{ money(total) }}</strong></div><el-button type="primary" size="large" :disabled="count === 0" @click="continueCheckout">确认并填写购票人</el-button><p>提交订单后将短暂锁定所选座位，请及时完成支付。</p>
        </aside>
      </div>
    </template>
    <EmptyState v-else title="票档信息不可用" description="请返回节目详情重新选择。"><el-button type="primary" @click="router.push(`/programs/${route.params.id}`)">返回详情</el-button></EmptyState>
  </section>
</template>

<style scoped>
.back-link { display: flex; align-items: center; gap: 6px; margin-bottom: 22px; padding: 0; border: 0; background: transparent; color: var(--muted); cursor: pointer; }
.seat-heading { display: flex; align-items: end; justify-content: space-between; gap: 25px; margin-bottom: 24px; }
.legend { display: flex; gap: 15px; color: var(--muted); font-size: 12px; }
.legend span { display: flex; align-items: center; gap: 5px; }
.legend i { width: 15px; height: 15px; border: 1px solid #b8bdc5; border-radius: 4px 4px 2px 2px; }
.legend i.selected { border-color: var(--accent); background: var(--accent); }.legend i.unavailable { border-color: #d6d9de; background: #d6d9de; }
.seat-layout { display: grid; grid-template-columns: minmax(0,1fr) 320px; gap: 24px; align-items: start; }
.seat-map { min-height: 520px; padding: 35px; }
.stage { width: min(76%, 520px); margin: 0 auto 70px; padding: 10px; border-top: 5px solid #8b919a; color: var(--muted); font-size: 12px; text-align: center; }
.seat-grid { width: min(620px,100%); display: grid; grid-template-columns: repeat(var(--columns), minmax(36px, 52px)); justify-content: center; gap: 14px; margin: auto; }
.seat { aspect-ratio: 1; display: grid; place-items: center; padding: 2px; border: 1px solid #adb3bc; border-radius: 6px 6px 3px 3px; background: #fff; color: #525862; cursor: pointer; font-size: 11px; transition: .15s ease; }
.seat:hover:not(:disabled) { border-color: var(--accent); color: var(--accent); transform: translateY(-2px); }.seat.selected { border-color: var(--accent); background: var(--accent); color: #fff; }.seat.unavailable { border-color: #d8dbe0; background: #d8dbe0; color: #a0a5ac; cursor: not-allowed; }
.seat-tip { display: flex; align-items: center; justify-content: center; gap: 6px; margin-top: 60px; color: var(--muted); font-size: 12px; }
.seat-summary { position: sticky; top: 92px; display: grid; gap: 16px; padding: 22px; box-shadow: var(--shadow); }.seat-summary h2 { margin: 0 0 4px; font-size: 19px; }.summary-line { display: flex; justify-content: space-between; gap: 12px; color: var(--muted); font-size: 13px; }.summary-line strong { color: var(--ink); }.selected-list { display: flex; flex-wrap: wrap; gap: 6px; }.selected-list span { padding: 5px 7px; border-radius: 4px; background: var(--soft); color: #50555e; font-size: 12px; }.summary-total { display: flex; align-items: baseline; justify-content: space-between; padding-top: 16px; border-top: 1px solid var(--line); }.summary-total strong { color: var(--accent); font-size: 23px; }.seat-summary p { margin: 0; color: var(--muted); font-size: 11px; line-height: 1.6; text-align: center; }
.general-admission { min-height: 430px; display: flex; align-items: center; justify-content: center; flex-direction: column; text-align: center; }.general-icon { width: 66px; height: 66px; display: grid; place-items: center; border-radius: 50%; background: #e7f4f2; color: var(--teal); font-size: 29px; }.general-admission h2 { margin: 18px 0 6px; }.general-admission p { margin: 0 0 24px; color: var(--muted); }.general-admission :deep(.el-input-number) { display: none; }.quantity-controls { display: flex; align-items: center; gap: 18px; }.quantity-controls strong { min-width: 56px; font-size: 18px; }
.seat-loading { padding: 40px; }
@media (max-width: 820px) { .seat-layout { grid-template-columns: 1fr; }.seat-summary { position: static; grid-row: 1; }.seat-map { min-height: 430px; }.stage { margin-bottom: 48px; } }
@media (max-width: 600px) { .seat-heading { align-items: start; flex-direction: column; }.seat-map { min-height: 390px; padding: 24px 12px; }.stage { margin-bottom: 38px; }.seat-grid { grid-template-columns: repeat(var(--columns), minmax(34px, 45px)); gap: 9px; }.seat-tip { margin-top: 42px; }.seat-summary { padding: 18px; } }
</style>
