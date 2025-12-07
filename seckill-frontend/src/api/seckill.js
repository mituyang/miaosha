import axios from 'axios'
import JSEncrypt from 'jsencrypt'

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

// 响应拦截器：处理 401 错误
api.interceptors.response.use(
  response => response,
  error => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('user')
    }
    return Promise.reject(error)
  }
)

// RSA 公钥缓存
let cachedPublicKey = null

// 获取 RSA 公钥（强制刷新时传 true）
export const getPublicKey = async (forceRefresh = false) => {
  if (cachedPublicKey && !forceRefresh) return cachedPublicKey
  const res = await api.get('/user/public-key')
  if (res.data.code === 0) {
    cachedPublicKey = res.data.data.public_key
    return cachedPublicKey
  }
  throw new Error('获取公钥失败')
}

// 清除公钥缓存（后端重启时需要重新获取）
export const clearPublicKeyCache = () => {
  cachedPublicKey = null
}

// RSA 加密密码
export const encryptPassword = async (password) => {
  const publicKey = await getPublicKey()
  const encrypt = new JSEncrypt()
  encrypt.setPublicKey(publicKey)
  const encrypted = encrypt.encrypt(password)
  if (!encrypted) {
    throw new Error('密码加密失败')
  }
  return encrypted
}

// 发送注册验证码
export const sendVerifyCode = (email) => {
  return api.post('/user/send-code', { email })
}

// 发送登录验证码
export const sendLoginCode = (email) => {
  return api.post('/user/send-login-code', { email })
}

// 带自动重试的加密请求（处理后端重启导致的密钥不匹配）
const encryptedRequest = async (requestFn, password) => {
  try {
    const encryptedPassword = await encryptPassword(password)
    return await requestFn(encryptedPassword)
  } catch (error) {
    // 如果是解密错误（400），清除公钥缓存并重试一次
    if (error.response?.status === 400 && 
        error.response?.data?.message?.includes('解密')) {
      clearPublicKeyCache()
      await getPublicKey(true) // 强制刷新公钥
      const encryptedPassword = await encryptPassword(password)
      return await requestFn(encryptedPassword)
    }
    throw error
  }
}

// 用户注册（密码加密传输，支持自动重试）
export const register = async (username, password, email, code, nickname) => {
  return encryptedRequest(
    (encryptedPassword) => api.post('/user/register', { 
      username, 
      password: encryptedPassword, 
      email, 
      code, 
      nickname 
    }),
    password
  )
}

// 邮箱密码登录（密码加密传输，支持自动重试）
export const login = async (email, password) => {
  return encryptedRequest(
    (encryptedPassword) => api.post('/user/login', { 
      email, 
      password: encryptedPassword 
    }),
    password
  )
}

// 验证码登录
export const loginByCode = (email, code) => {
  return api.post('/user/login-by-code', { email, code })
}

// 发送重置密码验证码
export const sendResetCode = (email) => {
  return api.post('/user/send-reset-code', { email })
}

// 重置密码（密码加密传输，支持自动重试）
export const resetPassword = async (email, code, password) => {
  return encryptedRequest(
    (encryptedPassword) => api.post('/user/reset-password', { 
      email, 
      code, 
      password: encryptedPassword 
    }),
    password
  )
}

// 获取用户信息
export const getUserInfo = () => {
  return api.get('/user/info')
}

// 获取商品列表
export const getGoodsList = () => {
  return api.get('/seckill/goods')
}

// 获取秒杀记录（公开展示）
export const getSeckillRecords = () => {
  return api.get('/seckill/records')
}

// 执行秒杀
export const doSeckill = (goodsId) => {
  return api.post('/seckill/do', { goods_id: goodsId })
}

// 查询秒杀结果
export const getSeckillResult = (goodsId) => {
  return api.get('/seckill/result', { params: { goods_id: goodsId } })
}

// 获取我的订单列表（支持筛选和分页）
export const getMyOrders = (params = {}) => {
  return api.get('/seckill/my-orders', { params })
}

// 获取订单详情
export const getOrderDetail = (orderId) => {
  return api.get('/seckill/order-detail', { params: { order_id: orderId } })
}

// 支付订单
export const payOrder = (orderId) => {
  return api.post('/seckill/pay', { order_id: orderId })
}

// 取消订单
export const cancelOrder = (orderId) => {
  return api.post('/seckill/cancel', { order_id: orderId })
}

// 健康检查
export const healthCheck = () => {
  return api.get('/health')
}
