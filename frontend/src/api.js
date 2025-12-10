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

// 秒杀下单
export const doSeckill = (goodsId) => {
  return api.post('/seckill/buy', { goods_id: goodsId })
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

// 预热所有商品
export const warmUpAll = (adminSecret) => {
  return api.post('/admin/warmup', {}, {
    headers: { 'X-Admin-Secret': adminSecret }
  })
}
