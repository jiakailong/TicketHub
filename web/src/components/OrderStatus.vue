<script setup>
import { computed } from 'vue'
import { statusText } from '../utils/format.js'
const props = defineProps({ status: { type: String, default: '' } })
const type = computed(() => ({ NO_PAY: 'warning', PAY: 'success', CANCEL: 'info', REFUND: 'danger' }[props.status] || 'info'))
</script>

<template><el-tag :type="type" effect="light" size="small">{{ statusText(status) }}</el-tag></template>
