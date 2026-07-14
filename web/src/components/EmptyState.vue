<script setup>
import { Tickets } from '@element-plus/icons-vue'
defineProps({ title: { type: String, default: '这里还没有内容' }, description: { type: String, default: '' } })
</script>

<template>
  <div class="empty-state">
    <span class="empty-icon"><el-icon><Tickets /></el-icon></span>
    <h3>{{ title }}</h3>
    <p v-if="description">{{ description }}</p>
    <slot />
  </div>
</template>

<style scoped>
.empty-state { min-height: 300px; display: grid; place-items: center; align-content: center; padding: 40px 20px; text-align: center; }
.empty-icon { width: 54px; height: 54px; display: grid; place-items: center; border-radius: 50%; background: var(--soft); color: #777d86; font-size: 25px; }
h3 { margin: 16px 0 5px; font-size: 18px; }
p { max-width: 430px; margin: 0 0 18px; color: var(--muted); line-height: 1.6; }
</style>
