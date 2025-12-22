<template>
  <div class="page">
    <div class="page-header">
      <h1 class="page-title">秒杀商品</h1>
      <button class="btn btn-secondary" @click="refreshStock">
        <span v-if="refreshing">刷新中...</span>
        <span v-else>刷新库存</span>
      </button>
    </div>

    <!-- 商品列表 -->
    <div v-if="loading" class="loading">加载中...</div>
    
    <div v-else class="goods-grid">
      <div v-for="goods in goodsList" :key="goods.id" class="goods-card">
        <div class="goods-image">
          <span class="goods-icon">📱</span>
        </div>
        <div class="goods-info">
          <h3 class="goods-name">{{ goods.name }}</h3>
          <div class="goods-price">¥{{ goods.price.toFixed(2) }}</div>
          <div class="goods-stock">
            库存: 
            <span :class="goods.stock > 0 ? 'stock-available' : 'stock-empty'">
              {{ goods.stock > 0 ? goods.stock : '已售罄' }}
            </span>
          </div>
          <div class="quantity-selector">
            <label>数量:</label>
            <select v-model="goods.quantity" class="quantity-select">
              <option v-for="n in maxBuyLimit" :key="n" :value="n">{{ n }}</option>
            </select>
          </div>
          <button 
            class="btn btn-primary btn-block" 
            :disabled="goods.stock <= 0 || seckilling === goods.id"
            @click="handleSeckill(goods.id, goods.quantity)"
          >
            {{ seckilling === goods.id ? '抢购中...' : (goods.stock > 0 ? '立即抢购' : '已售罄') }}
          </button>
        </div>
      </div>
    </div>

    <!-- 管理区域 -->
    <div class="admin-panel">
      <h3 class="admin-title">管理操作</h3>
      <div class="admin-form">
        <input 
          v-model="adminSecret" 
          type="password" 
          class="form-input" 
          placeholder="Admin Secret"
        />
        <button class="btn btn-success" @click="handleWarmUp" :disabled="warming">
          {{ warming ? '预热中...' : '库存预热' }}
        </button>
      </div>
    </div>

    <Toast ref="toast" />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { doSeckill, getStock, warmUpAll } from '../api'
import Toast from '../components/Toast.vue'

const toast = ref(null)
const loading = ref(false)
const refreshing = ref(false)
const seckilling = ref(null)
const warming = ref(false)
const adminSecret = ref('')
const maxBuyLimit = 5  // 最大限购数量

const goodsList = ref([
  { id: 1, name: 'iPhone 15 Pro', price: 6999, stock: 0, quantity: 1 },
  { id: 2, name: 'MacBook Pro M3', price: 12999, stock: 0, quantity: 1 }
])

const showToast = (message, type = 'info') => {
  toast.value?.show(message, type)
}

const refreshStock = async () => {
  refreshing.value = true
  try {
    for (const goods of goodsList.value) {
      const res = await getStock(goods.id)
      if (res.data.code === 0) {
        goods.stock = res.data.data.stock
      }
    }
    showToast('库存已刷新', 'success')
  } catch (e) {
    showToast('刷新失败', 'error')
  } finally {
    refreshing.value = false
  }
}

const handleSeckill = async (goodsId, quantity) => {
  // 本地拦截：库存为0直接返回，不发请求
  const goods = goodsList.value.find(g => g.id === goodsId)
  if (goods && goods.stock <= 0) {
    showToast('商品已售罄', 'error')
    return
  }

  // 检查数量
  if (quantity < 1 || quantity > maxBuyLimit) {
    showToast(`购买数量必须在 1-${maxBuyLimit} 之间`, 'error')
    return
  }

  seckilling.value = goodsId
  try {
    const res = await doSeckill(goodsId, quantity)
    if (res.data.code === 0) {
      showToast('秒杀请求已提交，请在订单页查看结果', 'success')
      await refreshStock()
    } else {
      // 售罄时更新本地库存为0
      if (res.data.code === 1001 && goods) {
        goods.stock = 0
      }
      showToast(res.data.msg, 'error')
    }
  } catch (e) {
    showToast(e.response?.data?.msg || '请求失败', 'error')
  } finally {
    seckilling.value = null
  }
}

const handleWarmUp = async () => {
  if (!adminSecret.value) {
    showToast('请输入 Admin Secret', 'error')
    return
  }
  warming.value = true
  try {
    const res = await warmUpAll(adminSecret.value)
    if (res.data.code === 0) {
      showToast(`预热完成，共 ${res.data.data.count} 个商品`, 'success')
      await refreshStock()
    } else {
      showToast(res.data.msg, 'error')
    }
  } catch (e) {
    showToast(e.response?.data?.msg || '预热失败', 'error')
  } finally {
    warming.value = false
  }
}

onMounted(() => {
  refreshStock()
})
</script>

<style scoped>
.quantity-selector {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 8px 0;
}

.quantity-selector label {
  font-size: 14px;
  color: #666;
}

.quantity-select {
  padding: 4px 8px;
  border: 1px solid #ddd;
  border-radius: 4px;
  font-size: 14px;
  min-width: 60px;
}
</style>
