<template>
  <div class="page orders-page">
    <div class="page-header">
      <div>
        <h1 class="page-title">我的订单</h1>
        <p class="page-subtitle">集中查看支付、取消和已完成订单，订单状态会在支付动作后实时刷新。</p>
      </div>
      <button class="btn btn-secondary" type="button" @click="fetchOrders" :disabled="loading">
        {{ loading ? '刷新中...' : '刷新订单' }}
      </button>
    </div>

    <nav class="order-tabs" aria-label="订单状态筛选">
      <button
        v-for="tab in tabs"
        :key="tab.value"
        type="button"
        :class="['order-tab', { active: activeTab === tab.value }]"
        @click="activeTab = tab.value"
      >
        <span>{{ tab.label }}</span>
        <span v-if="getTabCount(tab.value) > 0" class="order-tab-count">{{ getTabCount(tab.value) }}</span>
      </button>
    </nav>

    <div v-if="loading && orders.length === 0" class="loading-state">
      <div class="loading-spinner"></div>
      <p>订单加载中...</p>
    </div>

    <div v-else-if="filteredOrders.length === 0" class="empty-state order-empty">
      <h3>暂无相关订单</h3>
      <p>完成抢购后，新的订单会自动展示在这里。</p>
      <router-link to="/seckill" class="btn btn-primary">前往秒杀页</router-link>
    </div>

    <div v-else class="order-list">
      <article v-for="order in filteredOrders" :key="order.ID" class="order-card">
        <header class="order-card-header">
          <div class="order-meta">
            <span class="order-time">{{ formatTime(order.CreateTime) }}</span>
            <span class="order-no">订单号 {{ order.ID }}</span>
          </div>
          <div :class="['order-status', `status-${order.Status}`]">
            {{ statusText(order.Status) }}
          </div>
        </header>

        <div class="order-card-body">
          <div class="product-info">
            <div class="product-mark">{{ productMonogram(order.goods_name || `商品${order.GoodsID}`) }}</div>
            <div class="product-detail">
              <h3 class="product-name">{{ order.goods_name || `商品 ID: ${order.GoodsID}` }}</h3>
              <p class="product-spec">{{ order.activity_title || '默认活动' }} · 数量 {{ order.Quantity || 1 }} 件</p>
            </div>
          </div>

          <div class="product-quantity">
            <span class="quantity-value">{{ order.Quantity || 1 }}</span>
            <span class="quantity-label">件商品</span>
          </div>

          <div class="product-price">
            <span class="price-symbol">¥</span>
            <span class="price-value">{{ order.PayAmount.toFixed(2) }}</span>
          </div>
        </div>

        <footer class="order-card-footer">
          <div class="order-summary">
            <span>创建于 {{ formatTime(order.CreateTime) }}</span>
            <span v-if="order.Status !== 2">应付金额 {{ order.PayAmount.toFixed(2) }}</span>
          </div>

          <div class="order-actions">
            <template v-if="order.Status === 0">
              <button
                class="btn btn-primary"
                type="button"
                :disabled="processing === order.ID"
                @click="handlePay(order.ID)"
              >
                {{ processing === order.ID ? '处理中...' : '立即付款' }}
              </button>
              <button
                class="btn btn-secondary"
                type="button"
                :disabled="processing === order.ID"
                @click="handleCancel(order.ID)"
              >
                取消订单
              </button>
            </template>
            <template v-else-if="order.Status === 1">
              <router-link to="/seckill" class="btn btn-secondary">继续选购</router-link>
            </template>
          </div>
        </footer>
      </article>
    </div>

    <Toast ref="toast" />
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { cancelOrder, getOrders, payOrder } from '../api'
import Toast from '../components/Toast.vue'

const toast = ref(null)
const loading = ref(false)
const processing = ref(null)
const orders = ref([])
const activeTab = ref('all')

const tabs = [
  { label: '全部订单', value: 'all' },
  { label: '待付款', value: 'unpaid' },
  { label: '已付款', value: 'paid' },
  { label: '已取消', value: 'cancelled' }
]

const showToast = (message, type = 'info') => {
  toast.value?.show(message, type)
}

const statusText = status => {
  const map = { 0: '待付款', 1: '已付款', 2: '已取消' }
  return map[status] || '未知'
}

const formatTime = time => {
  if (!time) return '-'
  const date = new Date(time)
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}

const productMonogram = name => {
  const text = String(name || '').trim()
  if (!text) return 'SP'
  const latinParts = text.match(/[A-Za-z0-9]+/g)
  if (latinParts?.length) {
    return latinParts.slice(0, 2).map(part => part[0]).join('').toUpperCase()
  }
  return text.slice(0, 2)
}

const filteredOrders = computed(() => {
  if (activeTab.value === 'all') return orders.value
  const statusMap = { unpaid: 0, paid: 1, cancelled: 2 }
  return orders.value.filter(order => order.Status === statusMap[activeTab.value])
})

const getTabCount = tab => {
  if (tab === 'all') return orders.value.length
  const statusMap = { unpaid: 0, paid: 1, cancelled: 2 }
  return orders.value.filter(order => order.Status === statusMap[tab]).length
}

const fetchOrders = async () => {
  loading.value = true
  try {
    const res = await getOrders()
    if (res.data.code === 0) {
      orders.value = res.data.data || []
    } else {
      showToast(res.data.msg || '获取订单失败', 'error')
    }
  } catch (error) {
    showToast(error.response?.data?.msg || '获取订单失败', 'error')
  } finally {
    loading.value = false
  }
}

const handlePay = async orderId => {
  processing.value = orderId
  try {
    const res = await payOrder(orderId)
    if (res.data.code === 0) {
      showToast('支付成功', 'success')
      await fetchOrders()
    } else {
      showToast(res.data.msg || '支付失败', 'error')
    }
  } catch (error) {
    showToast(error.response?.data?.msg || '支付失败', 'error')
  } finally {
    processing.value = null
  }
}

const handleCancel = async orderId => {
  processing.value = orderId
  try {
    const res = await cancelOrder(orderId)
    if (res.data.code === 0) {
      showToast('订单已取消', 'success')
      await fetchOrders()
    } else {
      showToast(res.data.msg || '取消失败', 'error')
    }
  } catch (error) {
    showToast(error.response?.data?.msg || '取消失败', 'error')
  } finally {
    processing.value = null
  }
}

onMounted(() => {
  fetchOrders()
})
</script>

<style scoped>
.orders-page {
  max-width: 1200px;
}

.order-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-bottom: 22px;
}

.order-tab {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  min-height: 44px;
  padding: 0 16px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.72);
  color: var(--text-secondary);
  font-size: 16px;
  font-weight: 600;
}

.order-tab:hover {
  background: rgba(255, 255, 255, 0.92);
  color: var(--text-primary);
}

.order-tab.active {
  border-color: transparent;
  background: var(--text-primary);
  color: #ffffff;
  box-shadow: 0 16px 30px rgba(23, 23, 23, 0.14);
}

.order-tab-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 24px;
  height: 24px;
  padding: 0 6px;
  border-radius: 999px;
  background: rgba(179, 139, 58, 0.16);
  color: inherit;
  font-size: 14px;
  font-weight: 700;
}

.order-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.order-card {
  border: 1px solid var(--border);
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.84);
  box-shadow: var(--shadow-sm);
  overflow: hidden;
  backdrop-filter: blur(16px);
}

.order-card-header,
.order-card-body,
.order-card-footer {
  padding: 18px 22px;
}

.order-card-header,
.order-card-body {
  border-bottom: 1px solid rgba(23, 23, 23, 0.06);
}

.order-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.order-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  color: var(--text-muted);
  font-size: 15px;
}

.order-no {
  color: var(--text-secondary);
  font-weight: 600;
}

.order-status {
  display: inline-flex;
  align-items: center;
  min-height: 32px;
  padding: 0 12px;
  border-radius: 999px;
  font-size: 15px;
  font-weight: 700;
}

.order-status.status-0 {
  background: var(--warning-soft);
  color: var(--warning);
}

.order-status.status-1 {
  background: var(--success-soft);
  color: var(--success);
}

.order-status.status-2 {
  background: rgba(115, 115, 115, 0.12);
  color: var(--text-muted);
}

.order-card-body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 88px 160px;
  gap: 18px;
  align-items: center;
}

.product-info {
  display: flex;
  align-items: center;
  gap: 16px;
  min-width: 0;
}

.product-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 58px;
  height: 58px;
  border: 1px solid rgba(179, 139, 58, 0.18);
  border-radius: 18px;
  background: linear-gradient(180deg, rgba(243, 234, 215, 0.88) 0%, rgba(255, 255, 255, 0.94) 100%);
  color: var(--accent-strong);
  font-size: 18px;
  font-weight: 700;
  letter-spacing: 0.08em;
}

.product-detail {
  min-width: 0;
}

.product-name {
  color: var(--text-primary);
  font-size: 18px;
  font-weight: 700;
  line-height: 1.4;
}

.product-spec {
  margin-top: 4px;
  color: var(--text-secondary);
  font-size: 15px;
}

.product-quantity,
.product-price {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  justify-content: center;
}

.quantity-value {
  color: var(--text-primary);
  font-size: 22px;
  font-weight: 700;
}

.quantity-label,
.price-symbol {
  color: var(--text-muted);
  font-size: 14px;
}

.price-value {
  color: var(--text-primary);
  font-size: 28px;
  font-weight: 700;
  letter-spacing: -0.03em;
}

.order-card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.order-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  color: var(--text-secondary);
  font-size: 15px;
}

.order-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  justify-content: flex-end;
}

.order-empty {
  display: flex;
  flex-direction: column;
  gap: 18px;
  align-items: center;
}

@media (max-width: 840px) {
  .order-card-body {
    grid-template-columns: 1fr;
  }

  .product-quantity,
  .product-price {
    align-items: flex-start;
  }
}

@media (max-width: 640px) {
  .order-card-header,
  .order-card-body,
  .order-card-footer {
    padding: 16px;
  }

  .order-card-footer {
    flex-direction: column;
    align-items: stretch;
  }

  .order-actions {
    justify-content: stretch;
  }

  .order-actions .btn {
    width: 100%;
  }
}
</style>
