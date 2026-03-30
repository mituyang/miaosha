<template>
  <div class="page seckill-page">
    <div class="page-header">
      <div class="page-copy">
        <h1 class="page-title">秒杀商品</h1>
        <p class="page-subtitle">仅展示当前已上架商品，库存以 Redis 预热结果为准。</p>
        <div class="page-facts">
          <span class="page-fact">当前上架 {{ goodsList.length }} 款商品</span>
          <span class="page-fact">库存读取自 Redis</span>
        </div>
      </div>
      <button class="btn btn-secondary" @click="refreshStock" :disabled="refreshing || loading">
        {{ refreshing ? '刷新中...' : '刷新库存' }}
      </button>
    </div>

    <div v-if="loading" class="loading-state">
      <div class="loading-spinner"></div>
      <p>商品加载中...</p>
    </div>

    <div v-else-if="goodsList.length === 0" class="empty-state">
      <h3>暂无可售商品</h3>
      <p>管理员上架并预热库存后，这里会自动展示。</p>
      <button class="btn btn-secondary empty-action" @click="loadGoods">重新加载</button>
    </div>

    <div v-else class="goods-grid">
      <article v-for="goods in goodsList" :key="goods.id" class="goods-card">
        <div class="goods-card-head">
          <span class="goods-tag">秒杀</span>
          <span class="goods-stock-text" :class="goods.stock > 0 ? 'stock-available' : 'stock-empty'">
            {{ goods.stock > 0 ? `剩余 ${goods.stock}` : '已售罄' }}
          </span>
        </div>
        <div class="goods-info">
          <h3 class="goods-name">{{ goods.name }}</h3>
          <p class="goods-description">{{ goods.description || '暂无商品描述' }}</p>
          <div class="goods-price">¥{{ goods.price.toFixed(2) }}</div>
        </div>
        <div class="goods-actions">
          <div class="quantity-selector">
            <label :for="`quantity-${goods.id}`">购买数量</label>
            <select :id="`quantity-${goods.id}`" v-model="goods.quantity" class="quantity-select">
              <option v-for="n in maxBuyLimit" :key="n" :value="n">{{ n }}</option>
            </select>
          </div>
          <button
            class="btn btn-primary btn-block"
            :disabled="goods.stock <= 0 || seckilling === goods.id"
            @click="handleSeckill(goods.id, goods.quantity)"
          >
            {{ seckilling === goods.id ? '提交中...' : (goods.stock > 0 ? '立即抢购' : '已售罄') }}
          </button>
        </div>
      </article>
    </div>

    <Toast ref="toast" />
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { doSeckill, getGoodsList, getStock } from '../api'
import Toast from '../components/Toast.vue'

const toast = ref(null)
const loading = ref(false)
const refreshing = ref(false)
const seckilling = ref(null)
const maxBuyLimit = 5
const goodsList = ref([])

const showToast = (message, type = 'info') => {
  toast.value?.show(message, type)
}

const loadGoods = async () => {
  loading.value = true
  try {
    const res = await getGoodsList()
    if (res.data.code === 0) {
      goodsList.value = (res.data.data || []).map(item => ({
        id: item.ID,
        name: item.ProductName,
        description: item.Description,
        price: Number(item.Price || 0),
        stock: 0,
        quantity: 1
      }))
      await refreshStock()
    } else {
      showToast(res.data.msg || '加载商品失败', 'error')
    }
  } catch (e) {
    showToast(e.response?.data?.msg || '加载商品失败', 'error')
  } finally {
    loading.value = false
  }
}

const refreshStock = async () => {
  if (goodsList.value.length === 0) return
  refreshing.value = true
  try {
    const requests = goodsList.value.map(async goods => {
      const res = await getStock(goods.id)
      if (res.data.code === 0) {
        goods.stock = Number(res.data.data.stock || 0)
      }
    })
    await Promise.all(requests)
  } catch (e) {
    showToast('刷新库存失败', 'error')
  } finally {
    refreshing.value = false
  }
}

const handleSeckill = async (goodsId, quantity) => {
  const goods = goodsList.value.find(item => item.id === goodsId)
  if (!goods) return

  if (goods.stock <= 0) {
    showToast('商品已售罄', 'error')
    return
  }

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
      if (res.data.code === 1001) {
        goods.stock = 0
      }
      showToast(res.data.msg || '请求失败', 'error')
    }
  } catch (e) {
    showToast(e.response?.data?.msg || '请求失败', 'error')
  } finally {
    seckilling.value = null
  }
}

onMounted(() => {
  loadGoods()
})
</script>

<style scoped>
.seckill-page {
  max-width: 1120px;
}

.page-copy {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.page-facts {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.page-fact {
  display: inline-flex;
  align-items: center;
  min-height: 34px;
  padding: 0 14px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.74);
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 600;
}

.goods-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 20px;
}

.goods-card {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 24px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.96) 0%, rgba(248, 245, 238, 0.92) 100%);
  border: 1px solid var(--border);
  border-radius: 22px;
  box-shadow: var(--shadow-sm);
}

.goods-card-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.goods-tag {
  display: inline-flex;
  align-items: center;
  min-height: 32px;
  padding: 0 12px;
  border-radius: 999px;
  background: var(--accent-soft);
  color: var(--accent-strong);
  font-size: 13px;
  font-weight: 600;
}

.goods-stock-text {
  font-size: 13px;
  font-weight: 600;
}

.stock-available {
  color: var(--success);
}

.stock-empty {
  color: var(--danger);
}

.goods-info {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.goods-name {
  font-size: 20px;
  color: var(--text-primary);
  letter-spacing: -0.02em;
}

.goods-description {
  min-height: 44px;
  color: var(--text-secondary);
  font-size: 14px;
  line-height: 1.6;
}

.goods-price {
  font-size: 28px;
  font-weight: 700;
  color: var(--text-primary);
}

.goods-price::after {
  content: ' 限时价';
  margin-left: 6px;
  color: var(--accent-strong);
  font-size: 13px;
  font-weight: 600;
}

.goods-actions {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.quantity-selector {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.quantity-selector label {
  color: var(--text-secondary);
  font-size: 14px;
  font-weight: 600;
}

.quantity-select {
  min-width: 110px;
  min-height: 44px;
  padding: 0 12px;
  border: 1px solid var(--border);
  border-radius: 14px;
  font-size: 14px;
  background: rgba(255, 255, 255, 0.82);
  color: var(--text-primary);
}

.empty-action {
  margin-top: 18px;
}

@media (max-width: 640px) {
  .page-facts {
    gap: 8px;
  }

  .goods-card {
    padding: 20px;
  }

  .goods-name {
    font-size: 18px;
  }

  .goods-price {
    font-size: 24px;
  }

  .quantity-selector {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
