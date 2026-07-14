<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Calendar as CalendarDays, Location as MapPin, CircleCheck as ShieldCheck, Ticket, Avatar as Users, ArrowRight } from '@element-plus/icons-vue'
import { programApi } from '../api/index.js'
import { posterFor } from '../utils/posters.js'
import { cityText, dateTime, money, placeText, statusText } from '../utils/format.js'
import EmptyState from '../components/EmptyState.vue'

const route = useRoute()
const router = useRouter()
const loading = ref(true)
const program = ref(null)
const selectedCategory = ref('')
const category = computed(() => program.value?.ticket_categories?.find((item) => item.id === selectedCategory.value))

async function load() {
  loading.value = true
  try {
    program.value = await programApi.detail(route.params.id)
    selectedCategory.value = program.value.ticket_categories?.find((item) => item.remain > 0)?.id || program.value.ticket_categories?.[0]?.id || ''
  } finally { loading.value = false }
}

function chooseSeats() {
  if (!selectedCategory.value) return
  router.push({ name: 'seat-select', params: { id: route.params.id }, query: { category: selectedCategory.value } })
}

onMounted(load)
</script>

<template>
  <div v-if="loading" class="container page detail-loading"><el-skeleton :rows="8" animated /></div>
  <section v-else-if="program" class="detail-page">
    <div class="detail-band">
      <div class="container detail-grid">
        <div class="detail-poster"><img :src="posterFor(program.id)" :alt="`${program.title} 海报`" /><span>{{ statusText(program.status) }}</span></div>
        <div class="detail-content">
          <p class="eyebrow">{{ cityText(program.city) }} · {{ statusText(program.status) }}</p>
          <h1>{{ program.title }}</h1>
          <div class="facts"><p><el-icon><CalendarDays /></el-icon><span><small>演出时间</small>{{ dateTime(program.show_time) }}</span></p><p><el-icon><MapPin /></el-icon><span><small>演出场馆</small>{{ placeText(program.place) }}</span></p></div>
          <div class="assurances"><span><el-icon><ShieldCheck /></el-icon>官方票源</span><span><el-icon><Ticket /></el-icon>实名购票</span><span><el-icon><Users /></el-icon>一票一证</span></div>
        </div>
      </div>
    </div>

    <div class="container purchase-layout">
      <div class="event-notes">
        <div class="section-heading"><div><h2>购票须知</h2><p>请在提交前确认演出时间、票档与实名购票人。</p></div></div>
        <div class="notes-grid"><article><strong>实名规则</strong><p>每张门票需绑定一位有效证件购票人，入场信息须保持一致。</p></article><article><strong>出票说明</strong><p>订单支付后进入出票流程，最终状态以订单详情展示为准。</p></article><article><strong>退换政策</strong><p>演出票具有时效性，支付前请仔细核对，退款规则以主办方政策为准。</p></article><article><strong>儿童购票</strong><p>儿童入场政策因项目而异，请以现场实际执行规则为准。</p></article></div>
      </div>
      <aside class="purchase-panel surface">
        <div><p class="subtle-label">选择票档</p><div class="category-list"><button v-for="item in program.ticket_categories" :key="item.id" :disabled="item.remain <= 0" :class="{ selected: selectedCategory === item.id }" @click="selectedCategory = item.id"><span>{{ item.name }}</span><strong>{{ money(item.price_cent) }}</strong><small>{{ item.remain > 0 ? `余 ${item.remain}` : '已售罄' }}</small></button></div></div>
        <div class="purchase-total"><span>当前票档</span><strong>{{ category ? money(category.price_cent) : '请选择' }}</strong></div>
        <el-button type="primary" size="large" :disabled="!category || category.remain <= 0 || program.status !== 'ON_SALE'" @click="chooseSeats">选择座位<el-icon class="el-icon--right"><ArrowRight /></el-icon></el-button>
      </aside>
    </div>
  </section>
  <EmptyState v-else title="演出不存在" description="这场演出可能已下架或地址有误。"><RouterLink to="/"><el-button type="primary">返回首页</el-button></RouterLink></EmptyState>
</template>

<style scoped>
.detail-loading { max-width: 900px; }
.detail-band { padding: 44px 0; background: #25282d; color: #fff; }
.detail-grid { display: grid; grid-template-columns: 250px minmax(0, 1fr); gap: 48px; align-items: center; }
.detail-poster { position: relative; aspect-ratio: 3/4; overflow: hidden; border-radius: 8px; box-shadow: 0 18px 40px rgba(0,0,0,.28); }
.detail-poster img { width: 100%; height: 100%; object-fit: cover; }
.detail-poster span { position: absolute; left: 12px; top: 12px; padding: 6px 9px; border-radius: 4px; background: var(--accent); font-size: 12px; font-weight: 700; }
.detail-content h1 { max-width: 760px; margin: 0; font-size: 38px; line-height: 1.25; letter-spacing: 0; }
.facts { display: grid; gap: 18px; margin-top: 32px; }
.facts p { margin: 0; display: flex; gap: 13px; color: #f3f4f5; }
.facts .el-icon { margin-top: 4px; color: #f6abb4; font-size: 18px; }
.facts span { display: grid; gap: 5px; }
.facts small { color: #9fa5ad; }
.assurances { display: flex; flex-wrap: wrap; gap: 20px; margin-top: 34px; padding-top: 24px; border-top: 1px solid #44484f; color: #cdd0d5; font-size: 13px; }
.assurances span { display: flex; align-items: center; gap: 7px; }
.assurances .el-icon { color: #5bc1b6; }
.purchase-layout { display: grid; grid-template-columns: minmax(0,1fr) 360px; gap: 38px; align-items: start; padding-block: 44px 72px; }
.notes-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 1px; overflow: hidden; border: 1px solid var(--line); border-radius: 8px; background: var(--line); }
.notes-grid article { min-height: 138px; padding: 22px; background: #fff; }
.notes-grid strong { font-size: 15px; }
.notes-grid p { margin: 9px 0 0; color: var(--muted); font-size: 13px; line-height: 1.75; }
.purchase-panel { position: sticky; top: 92px; display: grid; gap: 24px; padding: 24px; box-shadow: var(--shadow); }
.category-list { display: grid; gap: 9px; margin-top: 10px; }
.category-list button { display: grid; grid-template-columns: 1fr auto; gap: 4px 12px; padding: 13px; border: 1px solid var(--line); border-radius: 6px; background: #fff; color: var(--ink); cursor: pointer; text-align: left; }
.category-list button strong { color: var(--accent); }
.category-list button small { grid-column: 1/-1; color: var(--muted); }
.category-list button.selected { border-color: var(--accent); background: #fff4f5; box-shadow: inset 3px 0 var(--accent); }
.category-list button:disabled { opacity: .5; cursor: not-allowed; }
.purchase-total { display: flex; justify-content: space-between; padding-top: 18px; border-top: 1px solid var(--line); }
.purchase-total span { color: var(--muted); }
.purchase-total strong { color: var(--accent); font-size: 21px; }
@media (max-width: 860px) { .detail-grid { grid-template-columns: 200px 1fr; gap: 30px; } .purchase-layout { grid-template-columns: 1fr; } .purchase-panel { position: static; grid-row: 1; } }
@media (max-width: 620px) { .detail-band { padding: 25px 0 30px; } .detail-grid { grid-template-columns: 115px minmax(0,1fr); gap: 18px; align-items: start; } .detail-content h1 { font-size: 23px; } .detail-content .eyebrow { margin-top: 2px; } .facts { margin-top: 17px; gap: 10px; } .facts p { font-size: 13px; } .facts small { display: none; } .assurances { grid-column: 1/-1; margin-top: 18px; padding-top: 16px; gap: 12px; } .purchase-layout { padding-block: 22px 46px; gap: 28px; } .purchase-panel { padding: 18px; } .notes-grid { grid-template-columns: 1fr; } .notes-grid article { min-height: auto; } }
</style>
