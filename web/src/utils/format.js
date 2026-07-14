const statusMap = {
  ON_SALE: '售票中',
  COMING_SOON: '即将开售',
  NO_PAY: '待支付',
  PAY: '已支付',
  CANCEL: '已取消',
  REFUND: '已退款',
}

export function money(value = 0) {
  return `¥${(Number(value) / 100).toLocaleString('zh-CN', { minimumFractionDigits: 0, maximumFractionDigits: 2 })}`
}

export function dateTime(value) {
  if (!value) return '时间待定'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit', weekday: 'short', hour: '2-digit', minute: '2-digit', hour12: false,
  }).format(date)
}

export const statusText = (value) => statusMap[value] || value || '未知'

const cityMap = { Shanghai: '上海', Beijing: '北京', Hangzhou: '杭州' }
const placeMap = {
  'Mercedes-Benz Arena': '梅赛德斯-奔驰文化中心',
  'National Stadium': '国家体育场',
  'Hangzhou Grand Theatre': '杭州大剧院',
}

export const cityText = (value) => cityMap[value] || value || '城市待定'
export const placeText = (value) => placeMap[value] || value || '场馆待定'

export function maskCertificate(value = '') {
  if (value.length < 8) return value
  return `${value.slice(0, 4)} **** **** ${value.slice(-4)}`
}
