<script setup>
import { computed, onMounted, ref } from 'vue'
import { Search, Location as MapPin, ArrowRight } from '@element-plus/icons-vue'
import { programApi } from '../api/index.js'
import ProgramCard from '../components/ProgramCard.vue'
import EmptyState from '../components/EmptyState.vue'
import { posterFor } from '../utils/posters.js'
import { cityText, dateTime, money, placeText } from '../utils/format.js'

const programs = ref([])
const loading = ref(true)
const keyword = ref('')
const city = ref('')
const cities = [{ label: '全部城市', value: 'ALL' }, { label: '上海', value: 'Shanghai' }, { label: '北京', value: 'Beijing' }, { label: '杭州', value: 'Hangzhou' }]
const featured = computed(() => programs.value.find((item) => item.status === 'ON_SALE') || programs.value[0])

async function load() {
  loading.value = true
  try {
    const data = await programApi.search({ keyword: keyword.value || undefined, city: city.value || undefined, page: 1, page_size: 24 })
    programs.value = data.programs || []
  } finally {
    loading.value = false
  }
}

function selectCity(value) {
  city.value = value === 'ALL' ? '' : value
  load()
}

onMounted(load)
</script>

<template>
  <section v-if="featured" class="featured" :style="{ backgroundImage: `url(${posterFor(featured.id)})` }">
    <div class="featured-mask"></div>
    <div class="container featured-inner">
      <p class="featured-kicker">本周焦点 · {{ cityText(featured.city) }}</p>
      <h1>{{ featured.title }}</h1>
      <p class="featured-meta">{{ dateTime(featured.show_time) }} · {{ placeText(featured.place) }}</p>
      <div class="featured-actions">
        <RouterLink :to="`/programs/${featured.id}`"><el-button type="primary" size="large">{{ money(featured.min_price_cent) }} 起 <el-icon class="el-icon--right"><ArrowRight /></el-icon></el-button></RouterLink>
      </div>
    </div>
  </section>
  <section v-else class="home-intro">
    <div class="container"><p class="eyebrow">TicketHub</p><h1>下一场现场，正在发生</h1><p>发现音乐、戏剧与值得奔赴的每一个夜晚。</p></div>
  </section>

  <section class="search-band">
    <form class="container search-bar" @submit.prevent="load">
      <el-input v-model="keyword" size="large" clearable placeholder="搜索演出、艺人或场馆" aria-label="搜索演出" @clear="load">
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
      <el-button native-type="submit" type="primary" size="large" :loading="loading">搜索</el-button>
    </form>
  </section>

  <section class="container programs-section">
    <div class="section-heading">
      <div><h2>正在热售</h2><p>从今天开始，安排一场真切的相遇。</p></div>
      <div class="city-filter desktop-only">
        <el-icon><MapPin /></el-icon>
        <button v-for="item in cities" :key="item.value" :class="{ active: city === (item.value === 'ALL' ? '' : item.value) }" @click="selectCity(item.value)">{{ item.label }}</button>
      </div>
    </div>

    <div class="mobile-city mobile-only">
      <el-select :model-value="city || 'ALL'" aria-label="选择城市" @change="selectCity"><el-option v-for="item in cities" :key="item.value" :label="item.label" :value="item.value" /></el-select>
    </div>

    <div v-if="loading" class="program-grid">
      <div v-for="index in 8" :key="index" class="program-skeleton"><el-skeleton animated><template #template><el-skeleton-item variant="image" class="skeleton-image" /><el-skeleton-item variant="h3" /><el-skeleton-item variant="text" /><el-skeleton-item variant="text" /></template></el-skeleton></div>
    </div>
    <div v-else-if="programs.length" class="program-grid"><ProgramCard v-for="program in programs" :key="program.id" :program="program" /></div>
    <EmptyState v-else title="没有找到匹配的演出" description="换一个关键词或城市，再看看新的现场。"><el-button @click="keyword = ''; city = ''; load()">查看全部演出</el-button></EmptyState>
  </section>
</template>

<style scoped>
.featured { position: relative; min-height: 430px; background-position: center 28%; background-size: cover; color: #fff; overflow: hidden; }
.featured-mask { position: absolute; inset: 0; background: rgba(11, 13, 16, .64); }
.featured-inner { position: relative; min-height: 430px; display: flex; justify-content: center; flex-direction: column; padding-block: 58px 84px; }
.featured-kicker { margin: 0 0 12px; color: #f3b5bd; font-size: 13px; font-weight: 700; }
.featured h1 { max-width: 700px; margin: 0; font-size: 48px; line-height: 1.15; letter-spacing: 0; }
.featured-meta { margin: 18px 0 0; color: #e4e5e8; font-size: 16px; }
.featured-actions { margin-top: 28px; }
.home-intro { padding: 72px 0; background: #24272c; color: #fff; }
.home-intro h1 { margin: 0; font-size: 44px; }
.home-intro p:last-child { color: #bbc0c7; }
.search-band { position: relative; z-index: 2; margin-top: -35px; }
.search-bar { display: grid; grid-template-columns: 1fr 112px; gap: 10px; padding: 16px; background: #fff; border: 1px solid var(--line); border-radius: 8px; box-shadow: var(--shadow); }
.programs-section { padding: 52px 0 70px; }
.program-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 22px; }
.city-filter { display: flex; align-items: center; gap: 5px; color: var(--muted); }
.city-filter button { padding: 6px 9px; border: 0; border-radius: 5px; background: transparent; color: var(--muted); cursor: pointer; }
.city-filter button.active { color: var(--accent); background: #fff0f2; font-weight: 700; }
.program-skeleton { padding: 0 0 16px; overflow: hidden; background: #fff; border: 1px solid var(--line); border-radius: 8px; }
.program-skeleton :deep(.el-skeleton__item) { margin: 7px 14px 0; width: calc(100% - 28px); }
.program-skeleton :deep(.skeleton-image) { width: 100%; height: auto; aspect-ratio: 3/4; margin: 0 0 10px; border-radius: 0; }
.mobile-city { justify-content: flex-end; margin: -4px 0 16px; }
@media (max-width: 960px) { .program-grid { grid-template-columns: repeat(3, minmax(0,1fr)); } }
@media (max-width: 760px) { .featured, .featured-inner { min-height: 390px; } .featured { background-position: center 18%; } .featured-inner { justify-content: end; padding-block: 54px 70px; } .featured h1 { font-size: 34px; } .featured-meta { font-size: 14px; line-height: 1.6; } .search-band { margin-top: -26px; } .search-bar { grid-template-columns: 1fr; padding: 11px; } .search-bar .el-button { width: 100%; } .programs-section { padding-top: 38px; } .program-grid { grid-template-columns: repeat(2, minmax(0,1fr)); gap: 13px; } }
@media (max-width: 420px) { .program-grid { grid-template-columns: 1fr 1fr; } }
</style>
