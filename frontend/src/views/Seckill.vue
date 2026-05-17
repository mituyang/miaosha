<template>
  <div class="page seckill-page">
    <div class="page-header">
      <div class="page-copy">
        <h1 class="page-title">秒杀活动</h1>
        <p class="page-subtitle">仅展示当前可见活动，库存以 Redis 活动预热结果为准。</p>
        <div class="page-facts">
          <span class="page-fact">当前活动 {{ activityList.length }} 场</span>
          <span class="page-fact">按活动隔离库存与限购</span>
        </div>
      </div>
      <button class="btn btn-secondary" @click="refreshStock" :disabled="refreshing || loading">
        {{ refreshing ? '刷新中...' : '刷新库存' }}
      </button>
    </div>

    <div v-if="loading" class="loading-state">
      <div class="loading-spinner"></div>
      <p>活动加载中...</p>
    </div>

    <div v-else-if="activityList.length === 0" class="empty-state">
      <h3>暂无秒杀活动</h3>
      <p>管理员创建活动并预热库存后，这里会自动展示。</p>
      <button class="btn btn-secondary empty-action" @click="loadActivities">重新加载</button>
    </div>

    <div v-else class="goods-grid">
      <article v-for="activity in activityList" :key="activity.id" class="goods-card">
        <div class="goods-card-head">
          <span class="goods-tag">{{ activityStatusText(activity) }}</span>
          <span class="goods-stock-text" :class="activity.stock > 0 ? 'stock-available' : 'stock-empty'">
            {{ activity.stock > 0 ? `剩余 ${activity.stock}` : '已售罄' }}
          </span>
        </div>
        <div class="goods-info">
          <h3 class="goods-name">{{ activity.title }}</h3>
          <p class="goods-description">{{ activity.goodsDescription || '暂无商品描述' }}</p>
          <div class="goods-meta">{{ activity.goodsName }} · 限购 {{ activity.maxBuyLimit }} 件</div>
          <div class="goods-time">{{ formatActivityTime(activity.startTime) }} - {{ formatActivityTime(activity.endTime) }}</div>
          <div class="goods-price">¥{{ activity.goodsPrice.toFixed(2) }}</div>
        </div>
        <div class="goods-actions">
          <div class="quantity-selector">
            <label :for="`quantity-${activity.id}`">购买数量</label>
            <select :id="`quantity-${activity.id}`" v-model="activity.quantity" class="quantity-select">
              <option v-for="n in activity.maxBuyLimit" :key="n" :value="n">{{ n }}</option>
            </select>
          </div>
          <button
            class="btn btn-primary btn-block"
            :disabled="!canBuy(activity) || seckilling === activity.id"
            @click="handleSeckill(activity.id, activity.quantity)"
          >
            {{ seckilling === activity.id ? '提交中...' : buyButtonText(activity) }}
          </button>
        </div>
      </article>
    </div>

    <Toast ref="toast" />
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { doSeckill, getActivityList, getActivityStock } from '../api'
import Toast from '../components/Toast.vue'

const toast = ref(null)
const loading = ref(false)
const refreshing = ref(false)
const seckilling = ref(null)
const activityList = ref([])

const showToast = (message, type = 'info') => {
  toast.value?.show(message, type)
}

const normalizeActivity = item => ({
  id: Number(item.id || 0),
  goodsId: Number(item.goods_id || 0),
  title: item.title || item.goods_name || '秒杀活动',
  goodsName: item.goods_name || '-',
  goodsDescription: item.goods_description || '',
  goodsPrice: Number(item.goods_price || 0),
  status: Number(item.status || 0),
  warmupStatus: Number(item.warmup_status || 0),
  maxBuyLimit: Math.max(Number(item.max_buy_limit || 1), 1),
  startTime: item.start_time,
  endTime: item.end_time,
  stock: 0,
  quantity: 1
})

const loadActivities = async () => {
  loading.value = true
  try {
    const res = await getActivityList()
    if (res.data.code === 0) {
      activityList.value = (res.data.data || []).map(normalizeActivity)
      await refreshStock()
    } else {
      showToast(res.data.msg || '加载活动失败', 'error')
    }
  } catch (e) {
    showToast(e.response?.data?.msg || '加载活动失败', 'error')
  } finally {
    loading.value = false
  }
}

const refreshStock = async () => {
  if (activityList.value.length === 0) return
  refreshing.value = true
  try {
    const requests = activityList.value.map(async activity => {
      const res = await getActivityStock(activity.id)
      if (res.data.code === 0) {
        activity.stock = Number(res.data.data.stock || 0)
      }
    })
    await Promise.all(requests)
  } catch (e) {
    showToast('刷新库存失败', 'error')
  } finally {
    refreshing.value = false
  }
}

const isInTimeWindow = activity => {
  const now = Date.now()
  const start = new Date(activity.startTime).getTime()
  const end = new Date(activity.endTime).getTime()
  return now >= start && now <= end
}

const canBuy = activity => {
  return activity.stock > 0 && activity.warmupStatus === 1 && isInTimeWindow(activity) && [0, 1].includes(activity.status)
}

const buyButtonText = activity => {
  if (activity.stock <= 0) return '已售罄'
  if (activity.warmupStatus !== 1) return '待预热'
  if (!isInTimeWindow(activity)) return '未开始'
  if (![0, 1].includes(activity.status)) return '已结束'
  return '立即抢购'
}

const activityStatusText = activity => {
  if (activity.status === 3) return '停用'
  if (activity.status === 2) return '已结束'
  const now = Date.now()
  if (now < new Date(activity.startTime).getTime()) return '未开始'
  if (now > new Date(activity.endTime).getTime()) return '已结束'
  return '秒杀中'
}

const formatActivityTime = value => {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hour = String(date.getHours()).padStart(2, '0')
  const minute = String(date.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day} ${hour}:${minute}`
}

const handleSeckill = async (activityId, quantity) => {
  const activity = activityList.value.find(item => item.id === activityId)
  if (!activity) return

  if (!canBuy(activity)) {
    showToast(buyButtonText(activity), 'error')
    return
  }

  if (quantity < 1 || quantity > activity.maxBuyLimit) {
    showToast(`购买数量必须在 1-${activity.maxBuyLimit} 之间`, 'error')
    return
  }

  seckilling.value = activityId
  try {
    const res = await doSeckill(activityId, quantity)
    if (res.data.code === 0) {
      showToast('秒杀请求已提交，请在订单页查看结果', 'success')
      await refreshStock()
    } else {
      if (res.data.code === 1001) {
        activity.stock = 0
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
  loadActivities()
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
}

.goods-description {
  min-height: 44px;
  color: var(--text-secondary);
  font-size: 14px;
  line-height: 1.6;
}

.goods-meta,
.goods-time {
  color: var(--text-muted);
  font-size: 13px;
  font-weight: 600;
}

.goods-price {
  font-size: 28px;
  font-weight: 700;
  color: var(--text-primary);
}

.goods-price::after {
  content: ' 活动价';
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
