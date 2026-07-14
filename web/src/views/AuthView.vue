<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Lock, Phone, ArrowRight } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { userApi } from '../api/index.js'
import { useAuthStore } from '../stores/auth.js'
import livePoster from '../assets/posters/live.jpg'

const props = defineProps({ mode: { type: String, default: 'login' } })
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const formRef = ref()
const loading = ref(false)
const isLogin = computed(() => props.mode === 'login')
const form = reactive({ mobile: '', password: '', confirmPassword: '' })

const rules = computed(() => ({
  mobile: [{ required: true, message: '请输入手机号', trigger: 'blur' }, { pattern: /^1\d{10}$/, message: '请输入 11 位手机号', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }, { min: 6, message: '密码至少 6 位', trigger: 'blur' }],
  confirmPassword: isLogin.value ? [] : [{ validator: (_, value, callback) => value === form.password ? callback() : callback(new Error('两次密码不一致')), trigger: 'blur' }],
}))

async function submit() {
  await formRef.value.validate()
  loading.value = true
  try {
    if (isLogin.value) {
      await auth.login({ mobile: form.mobile, password: form.password })
      ElMessage.success('欢迎回来')
      router.replace(String(route.query.redirect || '/'))
    } else {
      await userApi.register({ mobile: form.mobile, password: form.password })
      ElMessage.success('注册成功，请登录')
      router.replace({ name: 'login', query: { mobile: form.mobile } })
    }
  } finally {
    loading.value = false
  }
}

watch(() => route.query.mobile, (value) => { if (value) form.mobile = String(value) }, { immediate: true })
</script>

<template>
  <section class="auth-page">
    <div class="auth-image" :style="{ backgroundImage: `url(${livePoster})` }"><div class="auth-image-mask"></div><div class="auth-copy"><p>TicketHub 现场计划</p><h1>把期待，变成一张真实的票。</h1><span>从发现演出到选定座位，每一步都清楚可靠。</span></div></div>
    <div class="auth-form-wrap">
      <div class="auth-form">
        <p class="eyebrow">{{ isLogin ? 'Welcome back' : 'Join TicketHub' }}</p>
        <h2>{{ isLogin ? '登录 TicketHub' : '创建你的账户' }}</h2>
        <p class="form-lead">{{ isLogin ? '继续查看订单和未完成的购票。' : '注册后即可选座、管理购票人与订单。' }}</p>
        <el-form ref="formRef" :model="form" :rules="rules" label-position="top" size="large" @submit.prevent="submit">
          <el-form-item label="手机号" prop="mobile"><el-input v-model="form.mobile" :prefix-icon="Phone" autocomplete="tel" maxlength="11" placeholder="请输入手机号" /></el-form-item>
          <el-form-item label="密码" prop="password"><el-input v-model="form.password" :prefix-icon="Lock" type="password" show-password autocomplete="current-password" placeholder="至少 6 位" @keyup.enter="submit" /></el-form-item>
          <el-form-item v-if="!isLogin" label="确认密码" prop="confirmPassword"><el-input v-model="form.confirmPassword" :prefix-icon="Lock" type="password" show-password autocomplete="new-password" placeholder="再次输入密码" @keyup.enter="submit" /></el-form-item>
          <el-button class="submit-button" type="primary" native-type="submit" :loading="loading">{{ isLogin ? '登录' : '注册' }}<el-icon class="el-icon--right"><ArrowRight /></el-icon></el-button>
        </el-form>
        <p class="switch-auth">{{ isLogin ? '还没有账户？' : '已经有账户？' }} <RouterLink :to="isLogin ? '/register' : '/login'">{{ isLogin ? '立即注册' : '返回登录' }}</RouterLink></p>
      </div>
    </div>
  </section>
</template>

<style scoped>
.auth-page { min-height: calc(100vh - 68px); display: grid; grid-template-columns: minmax(360px, .9fr) minmax(480px, 1.1fr); background: #fff; }
.auth-image { position: relative; min-height: 660px; background-position: center; background-size: cover; color: #fff; }
.auth-image-mask { position: absolute; inset: 0; background: rgba(16,18,21,.62); }
.auth-copy { position: absolute; z-index: 1; left: clamp(32px, 7vw, 110px); right: 40px; bottom: 70px; }
.auth-copy p { color: #f3b5bd; font-weight: 700; }
.auth-copy h1 { max-width: 560px; margin: 10px 0 16px; font-size: 40px; line-height: 1.2; letter-spacing: 0; }
.auth-copy span { color: #d6d8dc; line-height: 1.7; }
.auth-form-wrap { display: grid; place-items: center; padding: 54px 40px; }
.auth-form { width: min(430px, 100%); }
.auth-form h2 { margin: 0; font-size: 30px; letter-spacing: 0; }
.form-lead { margin: 10px 0 30px; color: var(--muted); }
.submit-button { width: 100%; margin-top: 6px; }
.switch-auth { margin: 24px 0 0; color: var(--muted); text-align: center; }
.switch-auth a { color: var(--accent); font-weight: 700; }
@media (max-width: 760px) { .auth-page { min-height: calc(100vh - 60px); display: block; } .auth-image { min-height: 210px; } .auth-copy { left: 24px; right: 24px; bottom: 28px; } .auth-copy h1 { margin-bottom: 0; font-size: 27px; } .auth-copy span, .auth-copy p { display: none; } .auth-form-wrap { padding: 34px 20px 52px; } .auth-form h2 { font-size: 25px; } }
</style>
