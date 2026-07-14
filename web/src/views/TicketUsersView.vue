<script setup>
import { onMounted, reactive, ref } from 'vue'
import { Plus, User as UserRound, CreditCard, Phone } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { userApi } from '../api/index.js'
import { maskCertificate } from '../utils/format.js'
import EmptyState from '../components/EmptyState.vue'

const users = ref([])
const loading = ref(true)
const dialog = ref(false)
const submitting = ref(false)
const formRef = ref()
const form = reactive({ name: '', certificate_no: '', mobile: '' })
const rules = {
  name: [{ required: true, message: '请输入姓名', trigger: 'blur' }],
  certificate_no: [{ required: true, message: '请输入证件号码', trigger: 'blur' }, { min: 8, message: '证件号码格式不正确', trigger: 'blur' }],
  mobile: [{ required: true, message: '请输入手机号', trigger: 'blur' }, { pattern: /^1\d{10}$/, message: '请输入 11 位手机号', trigger: 'blur' }],
}

async function load() { loading.value = true; try { const data = await userApi.ticketUsers(); users.value = data.ticket_users || [] } finally { loading.value = false } }
function openDialog() { Object.assign(form, { name: '', certificate_no: '', mobile: '' }); dialog.value = true }
async function submit() { await formRef.value.validate(); submitting.value = true; try { await userApi.addTicketUser(form); ElMessage.success('购票人已添加'); dialog.value = false; await load() } finally { submitting.value = false } }
onMounted(load)
</script>

<template>
  <section class="container page">
    <div class="section-heading"><div><h1 class="page-title">常用购票人</h1><p class="page-lead">实名信息仅用于购票与入场核验，请确保与有效证件一致。</p></div><el-button type="primary" :icon="Plus" @click="openDialog">添加购票人</el-button></div>
    <div v-if="loading" class="user-grid"><div v-for="i in 3" :key="i" class="user-card surface"><el-skeleton :rows="3" animated /></div></div>
    <div v-else-if="users.length" class="user-grid"><article v-for="user in users" :key="user.id" class="user-card surface"><span class="avatar"><el-icon><UserRound /></el-icon></span><div><h2>{{ user.name }}</h2><p><el-icon><CreditCard /></el-icon>{{ maskCertificate(user.certificate_no) }}</p><small>实名购票人</small></div></article></div>
    <div v-else class="surface"><EmptyState title="还没有常用购票人" description="开始购票前，先添加一位实名购票人。"><el-button type="primary" :icon="Plus" @click="openDialog">添加第一位购票人</el-button></EmptyState></div>
  </section>
  <el-dialog v-model="dialog" title="添加购票人" width="min(92vw, 500px)" destroy-on-close>
    <el-form ref="formRef" :model="form" :rules="rules" label-position="top" size="large" @submit.prevent="submit"><el-form-item label="姓名" prop="name"><el-input v-model="form.name" :prefix-icon="UserRound" placeholder="与证件姓名一致" /></el-form-item><el-form-item label="证件号码" prop="certificate_no"><el-input v-model="form.certificate_no" :prefix-icon="CreditCard" placeholder="身份证或其他有效证件号码" /></el-form-item><el-form-item label="手机号" prop="mobile"><el-input v-model="form.mobile" :prefix-icon="Phone" maxlength="11" placeholder="用于接收票务通知" /></el-form-item></el-form>
    <template #footer><el-button @click="dialog = false">取消</el-button><el-button type="primary" :loading="submitting" @click="submit">确认添加</el-button></template>
  </el-dialog>
</template>

<style scoped>
.user-grid { display: grid; grid-template-columns: repeat(3,minmax(0,1fr)); gap: 18px; }.user-card { min-height: 146px; display: flex; align-items: flex-start; gap: 16px; padding: 22px; }.avatar { flex: 0 0 auto; width: 44px; height: 44px; display: grid; place-items: center; border-radius: 50%; background: #e7f4f2; color: var(--teal); font-size: 20px; }.user-card h2 { margin: 1px 0 12px; font-size: 17px; }.user-card p { display: flex; align-items: center; gap: 7px; margin: 0 0 10px; color: var(--muted); font-size: 13px; word-break: break-all; }.user-card small { color: var(--teal); font-weight: 600; }
@media (max-width: 820px) { .user-grid { grid-template-columns: 1fr 1fr; } }
@media (max-width: 580px) { .section-heading { align-items: stretch; flex-direction: column; }.user-grid { grid-template-columns: 1fr; }.user-card { min-height: 128px; } }
</style>
