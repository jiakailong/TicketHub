<template>
  <footer class="site-footer">
    <div class="container footer-inner">
      <div><strong>TicketHub</strong><span>连接现场，也连接每一次期待。</span></div>
      <span>票务服务 · 安全交易 · 实名购票</span>
    </div>
  </footer>
</template>

<style scoped>
.site-footer { border-top: 1px solid #292c31; background: #1e2125; color: #f4f5f6; }
.footer-inner { min-height: 112px; display: flex; align-items: center; justify-content: space-between; gap: 30px; }
.footer-inner div { display: grid; gap: 7px; }
.footer-inner strong { font-size: 18px; }
.footer-inner span { color: #aeb3bb; font-size: 13px; }
@media (max-width: 760px) { .footer-inner { min-height: 128px; align-items: start; justify-content: center; flex-direction: column; gap: 12px; } }
</style>
