import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/', name: 'home', component: () => import('../views/HomeView.vue'), meta: { title: '发现演出' } },
  { path: '/login', name: 'login', component: () => import('../views/AuthView.vue'), props: { mode: 'login' }, meta: { title: '登录' } },
  { path: '/register', name: 'register', component: () => import('../views/AuthView.vue'), props: { mode: 'register' }, meta: { title: '注册' } },
  { path: '/programs/:id', name: 'program-detail', component: () => import('../views/ProgramDetailView.vue'), meta: { title: '演出详情' } },
  { path: '/programs/:id/seats', name: 'seat-select', component: () => import('../views/SeatSelectView.vue'), meta: { title: '选择座位', requiresAuth: true } },
  { path: '/ticket-users', name: 'ticket-users', component: () => import('../views/TicketUsersView.vue'), meta: { title: '常用购票人', requiresAuth: true } },
  { path: '/checkout', name: 'checkout', component: () => import('../views/CheckoutView.vue'), meta: { title: '确认订单', requiresAuth: true } },
  { path: '/orders', name: 'orders', component: () => import('../views/OrdersView.vue'), meta: { title: '我的订单', requiresAuth: true } },
  { path: '/orders/:orderNumber', name: 'order-detail', component: () => import('../views/OrderDetailView.vue'), meta: { title: '订单详情', requiresAuth: true } },
  { path: '/account', name: 'account', component: () => import('../views/AccountView.vue'), meta: { title: '账户中心', requiresAuth: true } },
  { path: '/:pathMatch(.*)*', name: 'not-found', component: () => import('../views/NotFoundView.vue'), meta: { title: '页面不存在' } },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior: () => ({ top: 0 }),
})

router.beforeEach((to) => {
  document.title = `${to.meta.title || '演出购票'} | TicketHub`
  if (to.meta.requiresAuth && !localStorage.getItem('tickethub_token')) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
})

export default router
