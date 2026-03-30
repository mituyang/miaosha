import axios from 'axios'
import router from './router'

const api = axios.create({
  baseURL: '/api',
  timeout: 10000
})

// 请求拦截器：自动添加 Token
api.interceptors.request.use(config => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// 响应拦截器：处理 401
api.interceptors.response.use(
  res => res,
  err => {
    if (err.response?.status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('username')
      router.push('/login')
    }
    return Promise.reject(err)
  }
)

// 用户注册
export const register = (username, password) => {
  return api.post('/auth/register', { username, password })
}

// 用户登录
export const login = (username, password) => {
  return api.post('/auth/login', { username, password })
}

// 获取商品列表
export const getGoodsList = () => {
  return api.get('/goods')
}

// 秒杀下单
export const doSeckill = (goodsId, quantity) => {
  return api.post('/seckill/buy', { goods_id: goodsId, quantity: quantity })
}

// 查询库存
export const getStock = (goodsId) => {
  return api.get(`/seckill/stock/${goodsId}`)
}

// 获取订单列表
export const getOrders = () => {
  return api.get('/orders')
}

// 支付订单
export const payOrder = (orderId) => {
  return api.post(`/orders/${orderId}/pay`)
}

// 取消订单
export const cancelOrder = (orderId) => {
  return api.post(`/orders/${orderId}/cancel`)
}

const adminHeaders = (adminSecret) => ({
  'X-Admin-Secret': adminSecret
})

// 管理端密钥校验
export const adminPing = (adminSecret) => {
  return api.get('/admin/ping', {
    headers: adminHeaders(adminSecret)
  })
}

// 管理端查询商品
export const adminGetGoods = (adminSecret, params = {}) => {
  return api.get('/admin/goods', {
    params,
    headers: adminHeaders(adminSecret)
  })
}

// 管理端新增商品
export const adminCreateGoods = (adminSecret, payload) => {
  return api.post('/admin/goods', payload, {
    headers: adminHeaders(adminSecret)
  })
}

// 管理端更新商品
export const adminUpdateGoods = (adminSecret, goodsId, payload) => {
  return api.put(`/admin/goods/${goodsId}`, payload, {
    headers: adminHeaders(adminSecret)
  })
}

// 管理端删除商品
export const adminDeleteGoods = (adminSecret, goodsId) => {
  return api.delete(`/admin/goods/${goodsId}`, {
    headers: adminHeaders(adminSecret)
  })
}

// 管理端查询订单
export const adminGetOrders = (adminSecret, params = {}) => {
  return api.get('/admin/orders', {
    params,
    headers: adminHeaders(adminSecret)
  })
}

// 管理端查询订单详情
export const adminGetOrderDetail = (adminSecret, orderId) => {
  return api.get(`/admin/orders/${orderId}`, {
    headers: adminHeaders(adminSecret)
  })
}

// 管理端查询用户
export const adminGetUsers = (adminSecret, params = {}) => {
  return api.get('/admin/users', {
    params,
    headers: adminHeaders(adminSecret)
  })
}

// 管理端更新用户状态
export const adminUpdateUserStatus = (adminSecret, userId, status) => {
  return api.put(`/admin/users/${userId}/status`, { status }, {
    headers: adminHeaders(adminSecret)
  })
}

// 预热全部商品
export const adminWarmUpAll = (adminSecret) => {
  return api.post('/admin/warmup', {}, {
    headers: adminHeaders(adminSecret)
  })
}

// 预热单商品
export const adminWarmUpGoods = (adminSecret, goodsId) => {
  return api.post(`/admin/warmup/${goodsId}`, {}, {
    headers: adminHeaders(adminSecret)
  })
}

// 管理端查询统计数据
export const adminGetStats = (adminSecret) => {
  return api.get('/admin/stats', {
    headers: adminHeaders(adminSecret)
  })
}

// 管理端重建统计快照
export const adminRebuildStats = (adminSecret) => {
  return api.post('/admin/stats/rebuild', {}, {
    headers: adminHeaders(adminSecret)
  })
}
