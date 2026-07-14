<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { User as UserRound, Cellphone as Smartphone, CircleCheck as BadgeCheck, ShoppingBag, Avatar as UsersRound, SwitchButton as LogOut, Right as ChevronRight } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth.js'

const router = useRouter()
const auth = useAuthStore()
const loading = ref(false)
async function refresh() { loading.value = true; try { await auth.refresh() } finally { loading.value = false } }
function logout() { auth.logout(); router.push('/') }
onMounted(() => { if (!auth.user) refresh() })
</script>

<template>
  <section class="container page account-page">
    <div><p class="eyebrow">Account</p><h1 class="page-title">账户中心</h1><p class="page-lead">管理个人资料、实名购票人与订单。</p></div>
    <div class="account-layout">
      <section class="profile surface"><span class="profile-avatar"><el-icon><UserRound /></el-icon></span><div><h2>{{ auth.user?.mobile || 'TicketHub 用户' }}</h2><p>用户 ID {{ auth.user?.id || '-' }}</p><el-tag type="success" effect="light"><el-icon><BadgeCheck /></el-icon>{{ auth.user?.real_name_status === 'verified' ? '已实名' : '账户已验证' }}</el-tag></div><el-button :loading="loading" @click="refresh">刷新资料</el-button></section>
      <section class="quick-links surface"><button @click="router.push('/orders')"><span class="link-icon order"><el-icon><ShoppingBag /></el-icon></span><span><strong>我的订单</strong><small>查看支付与出票状态</small></span><el-icon><ChevronRight /></el-icon></button><button @click="router.push('/ticket-users')"><span class="link-icon people"><el-icon><UsersRound /></el-icon></span><span><strong>常用购票人</strong><small>维护实名证件信息</small></span><el-icon><ChevronRight /></el-icon></button><button @click="logout"><span class="link-icon logout"><el-icon><LogOut /></el-icon></span><span><strong>退出登录</strong><small>清除当前设备登录状态</small></span><el-icon><ChevronRight /></el-icon></button></section>
      <aside class="security surface"><h2>账户安全</h2><p><el-icon><Smartphone /></el-icon><span><strong>绑定手机号</strong><small>{{ auth.user?.mobile || '尚未加载' }}</small></span></p><div class="security-note"><strong>安全提示</strong><span>TicketHub 不会通过非官方渠道索要密码或验证码。</span></div></aside>
    </div>
  </section>
</template>

<style scoped>
.account-layout { display: grid; grid-template-columns: minmax(0,1fr) 320px; gap: 20px; align-items: start; margin-top: 28px; }.profile { grid-column: 1/-1; display: grid; grid-template-columns: 64px 1fr auto; align-items: center; gap: 18px; padding: 24px; }.profile-avatar { width: 64px; height: 64px; display: grid; place-items: center; border-radius: 50%; background: #e7f4f2; color: var(--teal); font-size: 29px; }.profile h2 { margin: 0 0 6px; font-size: 20px; }.profile p { margin: 0 0 9px; color: var(--muted); font-size: 12px; }.profile :deep(.el-tag__content) { display: flex; align-items: center; gap: 4px; }.quick-links { overflow: hidden; }.quick-links button { width: 100%; min-height: 86px; display: grid; grid-template-columns: 43px 1fr auto; align-items: center; gap: 14px; padding: 15px 19px; border: 0; border-bottom: 1px solid var(--line); background: #fff; color: var(--ink); cursor: pointer; text-align: left; }.quick-links button:last-child { border-bottom: 0; }.quick-links button:hover { background: #fafbfc; }.quick-links button > span:nth-child(2) { display: grid; gap: 5px; }.quick-links small { color: var(--muted); }.link-icon { width: 43px; height: 43px; display: grid; place-items: center; border-radius: 7px; font-size: 20px; }.link-icon.order { background: #fff0f2; color: var(--accent); }.link-icon.people { background: #e7f4f2; color: var(--teal); }.link-icon.logout { background: var(--soft); color: #626872; }.security { padding: 22px; }.security h2 { margin: 0 0 18px; font-size: 18px; }.security > p { display: flex; gap: 12px; margin: 0; }.security > p > .el-icon { margin-top: 3px; color: var(--blue); }.security > p span { display: grid; gap: 5px; }.security small { color: var(--muted); }.security-note { display: grid; gap: 7px; margin-top: 23px; padding: 14px; border-left: 3px solid var(--teal); background: #edf7f5; color: #356861; font-size: 12px; line-height: 1.6; }
@media (max-width: 720px) { .account-layout { grid-template-columns: 1fr; }.profile { grid-template-columns: 52px 1fr; }.profile-avatar { width: 52px; height: 52px; }.profile > .el-button { grid-column: 1/-1; }.security { grid-row: 3; } }
</style>
