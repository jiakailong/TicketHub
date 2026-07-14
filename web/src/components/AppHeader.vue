<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Menu, Ticket, User as UserRound, SwitchButton as LogOut, ShoppingBag, Avatar as UsersRound, House as Home } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth.js'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const drawer = ref(false)

const nav = [
  { label: '发现演出', to: '/', icon: Home },
  { label: '我的订单', to: '/orders', icon: ShoppingBag, auth: true },
  { label: '购票人', to: '/ticket-users', icon: UsersRound, auth: true },
]

function go(path) {
  drawer.value = false
  router.push(path)
}

function logout() {
  auth.logout()
  drawer.value = false
  router.push('/')
}
</script>

<template>
  <header class="site-header">
    <div class="container header-inner">
      <RouterLink class="brand" to="/" aria-label="TicketHub 首页">
        <span class="brand-mark"><el-icon><Ticket /></el-icon></span>
        <span>TicketHub</span>
      </RouterLink>

      <nav class="desktop-nav desktop-only" aria-label="主导航">
        <template v-for="item in nav" :key="item.to">
          <RouterLink v-if="!item.auth || auth.authenticated" :to="item.to" :class="{ active: route.path === item.to }">
            {{ item.label }}
          </RouterLink>
        </template>
      </nav>

      <div class="header-actions desktop-only">
        <template v-if="auth.authenticated">
          <el-button text :icon="UserRound" @click="go('/account')">{{ auth.user?.mobile || '账户' }}</el-button>
          <el-tooltip content="退出登录" placement="bottom">
            <el-button circle :icon="LogOut" aria-label="退出登录" @click="logout" />
          </el-tooltip>
        </template>
        <template v-else>
          <el-button text @click="go('/login')">登录</el-button>
          <el-button type="primary" @click="go('/register')">注册</el-button>
        </template>
      </div>

      <el-button class="mobile-menu mobile-only" circle :icon="Menu" aria-label="打开菜单" @click="drawer = true" />
    </div>
  </header>

  <el-drawer v-model="drawer" direction="rtl" size="min(82vw, 320px)" title="TicketHub">
    <nav class="drawer-nav" aria-label="移动端导航">
      <template v-for="item in nav" :key="item.to">
        <button v-if="!item.auth || auth.authenticated" :class="{ active: route.path === item.to }" @click="go(item.to)">
          <el-icon><component :is="item.icon" /></el-icon><span>{{ item.label }}</span>
        </button>
      </template>
      <button v-if="auth.authenticated" @click="go('/account')"><el-icon><UserRound /></el-icon><span>账户中心</span></button>
    </nav>
    <div class="drawer-account">
      <template v-if="auth.authenticated">
        <span>{{ auth.user?.mobile }}</span>
        <el-button :icon="LogOut" @click="logout">退出登录</el-button>
      </template>
      <template v-else>
        <el-button @click="go('/login')">登录</el-button>
        <el-button type="primary" @click="go('/register')">免费注册</el-button>
      </template>
    </div>
  </el-drawer>
</template>

<style scoped>
.site-header { position: sticky; top: 0; z-index: 30; height: 68px; background: rgba(255,255,255,.96); border-bottom: 1px solid var(--line); backdrop-filter: blur(12px); }
.header-inner { height: 100%; display: flex; align-items: center; gap: 40px; }
.brand { display: inline-flex; align-items: center; gap: 10px; font-size: 20px; font-weight: 800; white-space: nowrap; }
.brand-mark { width: 34px; height: 34px; display: grid; place-items: center; color: #fff; background: var(--accent); border-radius: 7px; font-size: 20px; }
.desktop-nav { display: flex; align-self: stretch; gap: 30px; }
.desktop-nav a { position: relative; display: flex; align-items: center; color: #545962; font-size: 14px; font-weight: 600; }
.desktop-nav a:hover, .desktop-nav a.active { color: var(--ink); }
.desktop-nav a.active::after { content: ''; position: absolute; left: 0; right: 0; bottom: -1px; height: 3px; background: var(--accent); }
.header-actions { margin-left: auto; display: flex; align-items: center; gap: 6px; }
.mobile-menu { margin-left: auto; }
.drawer-nav { display: grid; gap: 6px; }
.drawer-nav button { width: 100%; min-height: 48px; display: flex; align-items: center; gap: 13px; padding: 0 14px; border: 0; border-radius: 6px; background: transparent; color: #3e434b; cursor: pointer; text-align: left; }
.drawer-nav button.active { color: var(--accent); background: #fff0f2; font-weight: 700; }
.drawer-account { position: absolute; left: 20px; right: 20px; bottom: 24px; display: grid; gap: 10px; }
.drawer-account span { color: var(--muted); font-size: 13px; }
@media (max-width: 760px) { .site-header { height: 60px; } .header-inner { gap: 12px; } .brand { font-size: 18px; } .brand-mark { width: 32px; height: 32px; } }
</style>
