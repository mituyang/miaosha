import axios from 'axios'
import router from './router'

const api = axios.create({
  baseURL: '/api',
  timeout: 10000
})

// 请求拦截器：自动添加 Token
api.interceptors.request.use(config => {
  const token = localStorage.getItem('token')
  if (token && !config.headers?.Authorization && !config.headers?.authorization) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// 响应拦截器：处理 401
api.interceptors.response.use(
  res => res,
  err => {
    if (err.response?.status === 401) {
      if (err.config?.url?.startsWith('/admin')) {
        localStorage.removeItem('admin_token')
        return Promise.reject(err)
      }
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

// 获取登录验证码
export const getLoginCaptcha = () => {
  return api.get('/auth/captcha')
}

// 用户登录
export const login = (username, password, captchaId, captchaCode) => {
  return api.post('/auth/login', {
    username,
    password,
    captcha_id: captchaId,
    captcha_code: captchaCode
  })
}

// 获取商品列表
export const getGoodsList = () => {
  return api.get('/goods')
}

// 获取秒杀活动列表
export const getActivityList = () => {
  return api.get('/activities')
}

// 秒杀下单
export const doSeckill = (activityId, quantity) => {
  return api.post('/seckill/buy', { activity_id: activityId, quantity: quantity })
}

// 查询库存
export const getStock = (goodsId) => {
  return api.get(`/seckill/stock/${goodsId}`)
}

// 查询活动库存
export const getActivityStock = (activityId) => {
  return api.get(`/seckill/activity/${activityId}/stock`)
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

const adminHeaders = (adminToken) => ({
  Authorization: `Bearer ${adminToken}`
})

// 管理端登录
export const adminLogin = (username, password) => {
  return api.post('/admin/login', { username, password })
}

// 管理端登录态校验
export const adminPing = (adminToken) => {
  return api.get('/admin/ping', {
    headers: adminHeaders(adminToken)
  })
}

// 管理端查询商品
export const adminGetGoods = (adminToken, params = {}) => {
  return api.get('/admin/goods', {
    params,
    headers: adminHeaders(adminToken)
  })
}

// 管理端查询活动
export const adminGetActivities = (adminToken, params = {}) => {
  return api.get('/admin/activities', {
    params,
    headers: adminHeaders(adminToken)
  })
}

// 管理端新增活动
export const adminCreateActivity = (adminToken, payload) => {
  return api.post('/admin/activities', payload, {
    headers: adminHeaders(adminToken)
  })
}

// 管理端更新活动
export const adminUpdateActivity = (adminToken, activityId, payload) => {
  return api.put(`/admin/activities/${activityId}`, payload, {
    headers: adminHeaders(adminToken)
  })
}

// 管理端预热活动
export const adminWarmUpActivity = (adminToken, activityId) => {
  return api.post(`/admin/activities/${activityId}/warmup`, {}, {
    headers: adminHeaders(adminToken)
  })
}

// 管理端更新活动状态
export const adminUpdateActivityStatus = (adminToken, activityId, status) => {
  return api.put(`/admin/activities/${activityId}/status`, { status }, {
    headers: adminHeaders(adminToken)
  })
}

// 管理端新增商品
export const adminCreateGoods = (adminToken, payload) => {
  return api.post('/admin/goods', payload, {
    headers: adminHeaders(adminToken)
  })
}

// 管理端更新商品
export const adminUpdateGoods = (adminToken, goodsId, payload) => {
  return api.put(`/admin/goods/${goodsId}`, payload, {
    headers: adminHeaders(adminToken)
  })
}

// 管理端删除商品
export const adminDeleteGoods = (adminToken, goodsId) => {
  return api.delete(`/admin/goods/${goodsId}`, {
    headers: adminHeaders(adminToken)
  })
}

// 管理端查询订单
export const adminGetOrders = (adminToken, params = {}) => {
  return api.get('/admin/orders', {
    params,
    headers: adminHeaders(adminToken)
  })
}

// 管理端查询订单详情
export const adminGetOrderDetail = (adminToken, orderId) => {
  return api.get(`/admin/orders/${orderId}`, {
    headers: adminHeaders(adminToken)
  })
}

// 管理端查询用户
export const adminGetUsers = (adminToken, params = {}) => {
  return api.get('/admin/users', {
    params,
    headers: adminHeaders(adminToken)
  })
}

// 管理端更新用户状态
export const adminUpdateUserStatus = (adminToken, userId, status) => {
  return api.put(`/admin/users/${userId}/status`, { status }, {
    headers: adminHeaders(adminToken)
  })
}

// 预热全部商品
export const adminWarmUpAll = (adminToken) => {
  return api.post('/admin/warmup', {}, {
    headers: adminHeaders(adminToken)
  })
}

// 预热单商品
export const adminWarmUpGoods = (adminToken, goodsId) => {
  return api.post(`/admin/warmup/${goodsId}`, {}, {
    headers: adminHeaders(adminToken)
  })
}

// 管理端查询统计数据
export const adminGetStats = (adminToken) => {
  return api.get('/admin/stats', {
    headers: adminHeaders(adminToken)
  })
}

// 管理端重建统计快照
export const adminRebuildStats = (adminToken) => {
  return api.post('/admin/stats/rebuild', {}, {
    headers: adminHeaders(adminToken)
  })
}

// 管理端查询中间件运行监控数据
export const adminGetObservability = (adminToken) => {
  return api.get('/admin/observability', {
    headers: adminHeaders(adminToken)
  })
}
