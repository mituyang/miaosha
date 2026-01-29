<template>
  <div class="orders-page">
    <div class="orders-container">
      <!-- 页面标题 -->
      <div class="orders-header">
        <h1 class="orders-title">我的订单</h1>
        <button class="refresh-btn" @click="fetchOrders" :disabled="loading">
          <span class="refresh-icon">↻</span>
          {{ loading ? '刷新中...' : '刷新' }}
        </button>
      </div>

      <!-- 订单状态筛选 -->
      <div class="order-tabs">
        <div 
          v-for="tab in tabs" 
          :key="tab.value"
          :class="['tab-item', { active: activeTab === tab.value }]"
          @click="activeTab = tab.value"
        >
          {{ tab.label }}
          <span v-if="getTabCount(tab.value) > 0" class="tab-count">{{ getTabCount(tab.value) }}</span>
        </div>
      </div>

      <!-- 订单列表 -->
      <div v-if="loading && orders.length === 0" class="loading-state">
        <div class="loading-spinner"></div>
        <p>加载中...</p>
      </div>
      
      <div v-else-if="filteredOrders.length === 0" class="empty-state">
        <div class="empty-icon">📦</div>
        <p class="empty-text">暂无相关订单</p>
        <router-link to="/seckill" class="btn-go-shopping">去抢购</router-link>
      </div>
      
      <div v-else class="order-list">
        <div v-for="order in filteredOrders" :key="order.ID" class="order-card">
          <!-- 订单头部 -->
          <div class="order-card-header">
            <div class="order-meta">
              <span class="order-time">{{ formatTime(order.CreateTime) }}</span>
              <span class="order-no">订单号: {{ order.ID }}</span>
            </div>
            <div :class="['order-status', `status-${order.Status}`]">
              {{ statusText(order.Status) }}
            </div>
          </div>
          
          <!-- 订单商品 -->
          <div class="order-card-body">
            <div class="product-info">
              <div class="product-image">
                <span class="product-icon">{{ order.GoodsID === 1 ? '📱' : '💻' }}</span>
              </div>
              <div class="product-detail">
                <h3 class="product-name">{{ order.goods_name || `商品ID: ${order.GoodsID}` }}</h3>
                <p class="product-spec">官方正品 | 全国联保</p>
              </div>
            </div>
            <div class="product-quantity">
              <span class="quantity-label">x</span>
              <span class="quantity-value">{{ order.Quantity || 1 }}</span>
            </div>
            <div class="product-price">
              <span class="price-symbol">¥</span>
              <span class="price-value">{{ order.PayAmount.toFixed(2) }}</span>
            </div>
          </div>
          
          <!-- 订单底部 -->
          <div class="order-card-footer">
            <div class="order-summary" v-if="order.Status !== 2">
              共 <span class="highlight">{{ order.Quantity || 1 }}</span> 件商品，
              {{ order.Status === 0 ? '应付款' : '实付款' }}：<span class="total-price">¥{{ order.PayAmount.toFixed(2) }}</span>
            </div>
            <div class="order-actions">
              <template v-if="order.Status === 0">
                <button 
                  class="btn-action btn-pay" 
                  :disabled="processing === order.ID"
                  @click="handlePay(order.ID)"
                >
                  {{ processing === order.ID ? '处理中...' : '立即付款' }}
                </button>
                <button 
                  class="btn-action btn-cancel" 
                  :disabled="processing === order.ID"
                  @click="handleCancel(order.ID)"
                >
                  取消订单
                </button>
              </template>
              <template v-else-if="order.Status === 1">
                <button class="btn-action btn-review">评价</button>
                <button class="btn-action btn-rebuy">再次购买</button>
              </template>
            </div>
          </div>
        </div>
      </div>
    </div>

    <Toast ref="toast" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { getOrders, payOrder, cancelOrder } from '../api'
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

const statusText = (status) => {
  const map = { 0: '待付款', 1: '已付款', 2: '已取消' }
  return map[status] || '未知'
}

const formatTime = (time) => {
  if (!time) return '-'
  const d = new Date(time)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

const filteredOrders = computed(() => {
  if (activeTab.value === 'all') return orders.value
  const statusMap = { unpaid: 0, paid: 1, cancelled: 2 }
  return orders.value.filter(o => o.Status === statusMap[activeTab.value])
})

const getTabCount = (tab) => {
  if (tab === 'all') return orders.value.length
  const statusMap = { unpaid: 0, paid: 1, cancelled: 2 }
  return orders.value.filter(o => o.Status === statusMap[tab]).length
}

const fetchOrders = async () => {
  loading.value = true
  try {
    const res = await getOrders()
    if (res.data.code === 0) {
      orders.value = res.data.data || []
    }
  } catch (e) {
    console.error('获取订单失败', e)
  } finally {
    loading.value = false
  }
}

const handlePay = async (orderId) => {
  processing.value = orderId
  try {
    const res = await payOrder(orderId)
    if (res.data.code === 0) {
      showToast('支付成功', 'success')
      await fetchOrders()
    } else {
      showToast(res.data.msg, 'error')
    }
  } catch (e) {
    showToast(e.response?.data?.msg || '支付失败', 'error')
  } finally {
    processing.value = null
  }
}

const handleCancel = async (orderId) => {
  processing.value = orderId
  try {
    const res = await cancelOrder(orderId)
    if (res.data.code === 0) {
      showToast('订单已取消', 'success')
      await fetchOrders()
    } else {
      showToast(res.data.msg, 'error')
    }
  } catch (e) {
    showToast(e.response?.data?.msg || '取消失败', 'error')
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
  min-height: calc(100vh - 56px);
  background: #f5f5f5;
  padding: 20px 0;
}

.orders-container {
  max-width: 1200px;
  margin: 0 auto;
  padding: 0 20px;
}

.orders-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.orders-title {
  font-size: 22px;
  font-weight: 600;
  color: #333;
}

.refresh-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  background: #fff;
  border: 1px solid #ddd;
  border-radius: 4px;
  font-size: 14px;
  color: #666;
  cursor: pointer;
  transition: all 0.2s;
}

.refresh-btn:hover:not(:disabled) {
  border-color: #e4393c;
  color: #e4393c;
}

.refresh-icon {
  font-size: 14px;
}

/* 订单状态筛选 */
.order-tabs {
  display: flex;
  background: #fff;
  border-radius: 4px 4px 0 0;
  border-bottom: 2px solid #e4393c;
}

.tab-item {
  padding: 16px 32px;
  font-size: 14px;
  color: #666;
  cursor: pointer;
  position: relative;
  transition: all 0.2s;
}

.tab-item:hover {
  color: #e4393c;
}

.tab-item.active {
  color: #e4393c;
  font-weight: 600;
  background: #fff5f5;
}

.tab-item.active::after {
  content: '';
  position: absolute;
  bottom: -2px;
  left: 0;
  right: 0;
  height: 2px;
  background: #e4393c;
}

.tab-count {
  margin-left: 4px;
  padding: 2px 6px;
  background: #e4393c;
  color: #fff;
  font-size: 12px;
  border-radius: 10px;
}

/* 订单卡片 */
.order-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-top: 16px;
}

.order-card {
  background: #fff;
  border-radius: 8px;
  overflow: hidden;
  box-shadow: 0 1px 4px rgba(0,0,0,0.08);
}

.order-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  background: #fafafa;
  border-bottom: 1px solid #f0f0f0;
}

.order-meta {
  display: flex;
  align-items: center;
  gap: 20px;
  font-size: 13px;
  color: #999;
}

.order-no {
  color: #666;
}

.order-status {
  font-size: 14px;
  font-weight: 600;
  padding: 4px 12px;
  border-radius: 4px;
}

.order-status.status-0 {
  color: #e4393c;
  background: #fff1f0;
}

.order-status.status-1 {
  color: #52c41a;
  background: #f6ffed;
}

.order-status.status-2 {
  color: #999;
  background: #f5f5f5;
}

.order-card-body {
  display: flex;
  align-items: center;
  padding: 20px;
  border-bottom: 1px solid #f0f0f0;
}

.product-info {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 16px;
}

.product-image {
  width: 80px;
  height: 80px;
  background: #f8f8f8;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid #eee;
}

.product-icon {
  font-size: 40px;
}

.product-detail {
  flex: 1;
}

.product-name {
  font-size: 15px;
  font-weight: 500;
  color: #333;
  margin-bottom: 8px;
  line-height: 1.4;
}

.product-spec {
  font-size: 12px;
  color: #999;
}

.product-quantity {
  width: 80px;
  text-align: center;
  color: #666;
}

.quantity-label {
  font-size: 12px;
  color: #999;
}

.quantity-value {
  font-size: 16px;
  font-weight: 500;
}

.product-price {
  width: 140px;
  text-align: right;
  color: #333;
}

.price-symbol {
  font-size: 14px;
}

.price-value {
  font-size: 18px;
  font-weight: 600;
}

.order-card-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
}

.order-summary {
  font-size: 13px;
  color: #666;
}

.order-summary .highlight {
  color: #e4393c;
  font-weight: 600;
}

.total-price {
  font-size: 18px;
  font-weight: 600;
  color: #e4393c;
}

.order-actions {
  display: flex;
  gap: 12px;
}

.btn-action {
  padding: 8px 20px;
  border-radius: 4px;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s;
  border: 1px solid #ddd;
  background: #fff;
  color: #666;
}

.btn-action:hover:not(:disabled) {
  border-color: #e4393c;
  color: #e4393c;
}

.btn-action:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-pay {
  background: #e4393c;
  border-color: #e4393c;
  color: #fff;
}

.btn-pay:hover:not(:disabled) {
  background: #c9302c;
  border-color: #c9302c;
  color: #fff;
}

/* 空状态 */
.empty-state {
  text-align: center;
  padding: 80px 20px;
  background: #fff;
  border-radius: 8px;
  margin-top: 16px;
}

.empty-icon {
  font-size: 80px;
  margin-bottom: 20px;
}

.empty-text {
  font-size: 16px;
  color: #999;
  margin-bottom: 24px;
}

.btn-go-shopping {
  display: inline-block;
  padding: 12px 40px;
  background: #e4393c;
  color: #fff;
  text-decoration: none;
  border-radius: 4px;
  font-size: 14px;
  transition: background 0.2s;
}

.btn-go-shopping:hover {
  background: #c9302c;
}

/* 加载状态 */
.loading-state {
  text-align: center;
  padding: 80px 20px;
  background: #fff;
  border-radius: 8px;
  margin-top: 16px;
}

.loading-spinner {
  width: 40px;
  height: 40px;
  border: 3px solid #f0f0f0;
  border-top-color: #e4393c;
  border-radius: 50%;
  margin: 0 auto 16px;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.loading-state p {
  color: #999;
  font-size: 14px;
}
</style>
