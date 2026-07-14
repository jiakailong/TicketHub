import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { userApi } from '../api/index.js'

function readUser() {
  try {
    return JSON.parse(localStorage.getItem('tickethub_user') || 'null')
  } catch {
    return null
  }
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('tickethub_token') || '')
  const user = ref(readUser())
  const authenticated = computed(() => Boolean(token.value))

  async function login(credentials) {
    const data = await userApi.login(credentials)
    token.value = data.access_token
    user.value = data.user
    localStorage.setItem('tickethub_token', token.value)
    localStorage.setItem('tickethub_user', JSON.stringify(user.value))
    return data
  }

  async function refresh() {
    if (!token.value) return null
    user.value = await userApi.detail()
    localStorage.setItem('tickethub_user', JSON.stringify(user.value))
    return user.value
  }

  function logout() {
    token.value = ''
    user.value = null
    localStorage.removeItem('tickethub_token')
    localStorage.removeItem('tickethub_user')
    localStorage.removeItem('tickethub_checkout')
  }

  return { token, user, authenticated, login, refresh, logout }
})
