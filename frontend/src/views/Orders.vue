<template>
  <div class="page">
    <div class="page-header">
      <h1 class="page-title">我的订单</h1>
      <button class="btn btn-secondary" @click="fetchOrders">
        <span v-if="loading">刷新中...</span>
        <span v-else>刷新</span>
      </button>
    </div>

    <!-- 订单列表 -->
    <div v-if="loading && orders.length === 0" class="loading">加载中...</div>
    
    <div v-else-if="orders.length === 0" class="empty-state">
      <div class="empty-icon">📦</div>
      <p>暂无订单</p>
      <router-link to="/seckill" class="btn btn-primary">去抢购</router-link>
    </div>
    
    <div v-else class="order-list">
      <div v-for="order in orders" :key="order.ID" class="order-card">
        <div class="order-header">
          <span class="order-id">订单号: {{ order.ID }}</span>
          <span :class="['order-status', `status-${order.Status}`]">
            {{ statusText(order.Status) }}
          </span>
        </div>
        <div class="order-body">
          <div class="order-goods">{{ order.goods_name || `商品ID: ${order.GoodsID}` }}</div>
          <div class="order-amount">¥{{ order.PayAmount.toFixed(2) }}</div>
        </div>
        <div class="order-footer">
          <span class="order-time">{{ formatTime(order.CreateTime) }}</span>
          <!-- 待支付状态显示操作按钮 -->
          <div v-if="order.Status === 0" class="order-actions">
            <button 
              class="btn btn-primary btn-sm" 
              :disabled="processing === order.ID"
              @click="handlePay(order.ID)"
            >
              {{ processing === order.ID ? '处理中...' : '支付' }}
            </button>
            <button 
              class="btn btn-secondary btn-sm" 
              :disabled="processing === order.ID"
              @click="handleCancel(order.ID)"
            >
              取消
            </button>
          </div>
        </div>
      </div>
    </div>

    <Toast ref="toast" />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { getOrders, payOrder, cancelOrder } from '../api'
import Toast from '../components/Toast.vue'

const toast = ref(null)
const loading = ref(false)
const processing = ref(null)
const orders = ref([])

const showToast = (message, type = 'info') => {
  toast.value?.show(message, type)
}

const statusText = (status) => {
  const map = { 0: '待支付', 1: '已支付', 2: '已取消' }
  return map[status] || '未知'
}

const formatTime = (time) => {
  if (!time) return '-'
  return new Date(time).toLocaleString('zh-CN')
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
