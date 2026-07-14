import axios from 'axios'
import { ElMessage } from 'element-plus'

const client = axios.create({
  baseURL: '/api',
  timeout: 10000,
})

client.interceptors.request.use((config) => {
  const token = localStorage.getItem('tickethub_token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

client.interceptors.response.use(
  (response) => {
    const body = response.data
    if (body?.code && body.code !== 'OK') return Promise.reject(new Error(body.message || '请求失败'))
    return body?.data ?? body
  },
  (error) => {
    const status = error.response?.status
    const message = status === 401
      ? '登录状态已失效，请重新登录'
      : error.response?.data?.message || error.message || '网络开小差了，请稍后重试'
    if (status === 401) {
      localStorage.removeItem('tickethub_token')
      localStorage.removeItem('tickethub_user')
    }
    if (!error.config?.silent) ElMessage.error(message)
    return Promise.reject(error)
  },
)

export default client
