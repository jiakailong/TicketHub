import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

function readDraft() {
  try {
    return JSON.parse(sessionStorage.getItem('tickethub_checkout') || 'null')
  } catch {
    return null
  }
}

export const useCheckoutStore = defineStore('checkout', () => {
  const draft = ref(readDraft())
  const ticketCount = computed(() => draft.value?.seatIds?.length || draft.value?.quantity || 0)
  const amountCent = computed(() => ticketCount.value * (draft.value?.category?.price_cent || 0))

  function setDraft(value) {
    draft.value = value
    sessionStorage.setItem('tickethub_checkout', JSON.stringify(value))
  }

  function clear() {
    draft.value = null
    sessionStorage.removeItem('tickethub_checkout')
  }

  return { draft, ticketCount, amountCent, setDraft, clear }
})
