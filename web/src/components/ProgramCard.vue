<script setup>
import { Calendar as CalendarDays, Location as MapPin } from '@element-plus/icons-vue'
import { posterFor } from '../utils/posters.js'
import { cityText, dateTime, money, placeText, statusText } from '../utils/format.js'

defineProps({ program: { type: Object, required: true } })
</script>

<template>
  <RouterLink class="program-card" :to="`/programs/${program.id}`">
    <div class="poster-wrap">
      <img :src="posterFor(program.id)" :alt="`${program.title} 海报`" />
      <span class="program-badge">{{ statusText(program.status) }}</span>
    </div>
    <div class="program-info">
      <h3>{{ program.title }}</h3>
      <p><el-icon><CalendarDays /></el-icon><span>{{ dateTime(program.show_time) }}</span></p>
      <p><el-icon><MapPin /></el-icon><span>{{ cityText(program.city) }} · {{ placeText(program.place) }}</span></p>
      <div class="program-price"><span class="price">{{ money(program.min_price_cent) }}</span><small>起</small></div>
    </div>
  </RouterLink>
</template>

<style scoped>
.program-card { min-width: 0; overflow: hidden; background: #fff; border: 1px solid var(--line); border-radius: 8px; transition: transform .2s ease, box-shadow .2s ease; }
.program-card:hover { transform: translateY(-3px); box-shadow: var(--shadow); }
.poster-wrap { position: relative; aspect-ratio: 3 / 4; overflow: hidden; background: #e9ebee; }
.poster-wrap img { width: 100%; height: 100%; object-fit: cover; transition: transform .35s ease; }
.program-card:hover img { transform: scale(1.025); }
.program-badge { position: absolute; left: 10px; top: 10px; padding: 5px 8px; color: #fff; background: rgba(23,25,28,.86); border-radius: 4px; font-size: 12px; font-weight: 700; }
.program-info { padding: 15px 15px 17px; }
.program-info h3 { min-height: 46px; margin: 0 0 12px; font-size: 16px; line-height: 1.45; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
.program-info p { margin: 7px 0; display: flex; align-items: flex-start; gap: 7px; color: var(--muted); font-size: 12px; line-height: 1.45; }
.program-info .el-icon { flex: 0 0 auto; margin-top: 2px; }
.program-price { margin-top: 14px; display: flex; align-items: baseline; gap: 4px; }
.program-price .price { font-size: 19px; }
.program-price small { color: var(--muted); }
</style>
