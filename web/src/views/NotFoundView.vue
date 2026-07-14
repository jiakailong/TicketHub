<script setup>import { useRouter } from 'vue-router'; import EmptyState from '../components/EmptyState.vue'; const router = useRouter()</script>
<template><section class="container page"><div class="surface"><EmptyState title="页面走丢了" description="这个地址不存在，回到首页继续发现演出。"><el-button type="primary" @click="router.push('/')">返回首页</el-button></EmptyState></div></section></template>
