<template>
  <div class="container">
    <h1>🔥 秒杀系统</h1>
    
    <!-- Toast 通知 -->
    <Transition name="toast">
      <div v-if="toast.show" class="toast" :class="toast.type">
        <span class="toast-icon">{{ toastIcon }}</span>
        <span class="toast-msg">{{ toast.message }}</span>
      </div>
    </Transition>

    <!-- 用户状态栏 -->
    <div class="user-bar">
      <template v-if="isLoggedIn">
        <span>欢迎，{{ user.nickname || user.username }}</span>
        <button @click="handleLogout" class="logout-btn">退出登录</button>
      </template>
      <template v-else>
        <button @click="showLoginModal = true" class="login-btn">登录</button>
        <button @click="openRegisterModal" class="register-btn">注册</button>
      </template>
    </div>

    <!-- 秒杀记录滚动展示 -->
    <div v-if="seckillRecords.length > 0" class="seckill-records">
      <div class="records-title">🎉 秒杀战报</div>
      <div class="records-scroll">
        <div class="records-track">
          <span v-for="(record, index) in seckillRecords" :key="index" class="record-item">
            {{ record.goods_name }} · {{ record.nickname || '匿名用户' }}@{{ record.username }}
          </span>
        </div>
      </div>
    </div>

    <!-- 商品列表 -->
    <div class="goods-list">
      <div 
        v-for="goods in goodsList" 
        :key="goods.id" 
        class="goods-card"
        :class="{ 'sold-out': goods.stock <= 0 }"
      >
        <h3>{{ goods.name }}</h3>
        <p class="stock">库存：{{ goods.stock }}</p>
        <button 
          @click="handleSeckill(goods.id)"
          :disabled="loading || goods.stock <= 0"
          class="seckill-btn"
        >
          {{ goods.stock <= 0 ? '已售罄' : '立即秒杀' }}
        </button>
      </div>
    </div>

    <!-- 我的订单 -->
    <div v-if="isLoggedIn" class="my-orders">
      <div class="orders-header">
        <h2>📦 我的秒杀订单</h2>
        <div class="order-filters">
          <button 
            v-for="filter in orderFilters" 
            :key="filter.value"
            :class="['filter-btn', { active: orderStatus === filter.value }]"
            @click="changeOrderStatus(filter.value)"
          >
            {{ filter.label }}
          </button>
        </div>
      </div>
      <div v-if="myOrders.length > 0" class="orders-list">
        <div 
          v-for="order in myOrders" 
          :key="order.order_id" 
          class="order-card"
          :class="{ clickable: isOrderClickable(order) }"
          @click="isOrderClickable(order) && openPayModal(order.order_id)"
        >
          <div class="order-header">
            <span class="order-goods">{{ order.goods_name }}</span>
            <span class="order-status" :class="getStatusClass(order)">{{ getStatusText(order) }}</span>
          </div>
          <div class="order-info">
            <span class="order-id">订单号：{{ order.order_id }}</span>
            <span class="order-time">{{ order.created_at }}</span>
          </div>
          <div v-if="isOrderClickable(order)" class="order-tip">点击去支付</div>
        </div>
      </div>
      <div v-else class="no-orders">暂无订单</div>
      <!-- 分页 -->
      <div v-if="orderPagination.totalPages > 1" class="pagination">
        <button 
          :disabled="orderPagination.page <= 1" 
          @click="changeOrderPage(orderPagination.page - 1)"
          class="page-btn"
        >上一页</button>
        <span class="page-info">{{ orderPagination.page }} / {{ orderPagination.totalPages }}</span>
        <button 
          :disabled="orderPagination.page >= orderPagination.totalPages" 
          @click="changeOrderPage(orderPagination.page + 1)"
          class="page-btn"
        >下一页</button>
      </div>
    </div>

    <!-- 支付弹窗 -->
    <Transition name="modal">
      <div v-if="showPayModal" class="modal-overlay" @click="closePayModal">
        <div class="modal pay-modal" @click.stop>
          <button class="modal-close" @click="closePayModal">×</button>
          <h2>💳 订单支付</h2>
          <div v-if="payOrder" class="pay-info">
            <p class="pay-goods">{{ payOrder.goods_name }}</p>
            <p class="pay-order-id">订单号：{{ payOrder.order_id }}</p>
            <div v-if="payOrder.status === 0" class="pay-countdown">
              <span class="countdown-label">剩余支付时间</span>
              <span class="countdown-time">{{ formatCountdown(payCountdown) }}</span>
            </div>
            <div v-if="payOrder.status === 1" class="pay-success">
              <span class="success-icon">✓</span>
              <span>支付成功</span>
            </div>
            <div v-if="payOrder.status === 2" class="pay-cancelled">
              <span class="cancelled-icon">✕</span>
              <span>订单已取消</span>
            </div>
          </div>
          <div v-if="payOrder && payOrder.status === 0" class="pay-actions">
            <button 
              @click="handlePay" 
              :disabled="paying || cancelling || payCountdown <= 0"
              class="submit-btn pay-btn"
            >
              {{ paying ? '支付中...' : '立即支付' }}
            </button>
            <button 
              @click="handleCancel" 
              :disabled="paying || cancelling"
              class="submit-btn cancel-btn"
            >
              {{ cancelling ? '取消中...' : '取消订单' }}
            </button>
          </div>
          <button v-else @click="closePayModal" class="submit-btn">关闭</button>
        </div>
      </div>
    </Transition>

    <!-- 登录弹窗 -->
    <Transition name="modal">
      <div v-if="showLoginModal" class="modal-overlay" @click="showLoginModal = false">
        <div class="modal auth-modal" @click.stop>
          <button class="modal-close" @click="showLoginModal = false">×</button>
          <h2>👋 欢迎回来</h2>
          <p class="modal-subtitle">登录你的账号继续秒杀</p>
          
          <!-- 登录方式切换 -->
          <div class="login-tabs">
            <button :class="{ active: loginMode === 'password' }" @click="loginMode = 'password'">密码登录</button>
            <button :class="{ active: loginMode === 'code' }" @click="loginMode = 'code'">验证码登录</button>
          </div>
          
          <input v-model="loginForm.email" type="email" placeholder="邮箱地址" @keyup.enter="handleLogin" />
          
          <!-- 密码登录 -->
          <template v-if="loginMode === 'password'">
            <input v-model="loginForm.password" type="password" placeholder="密码" @keyup.enter="handleLogin" />
          </template>
          
          <!-- 验证码登录 -->
          <template v-else>
            <div class="code-row">
              <input v-model="loginForm.code" type="text" placeholder="验证码" maxlength="6" @keyup.enter="handleLogin" />
              <button 
                @click="handleSendLoginCode" 
                :disabled="loginCodeSending || loginCodeCountdown > 0"
                class="send-code-btn"
              >
                {{ loginCodeCountdown > 0 ? `${loginCodeCountdown}s` : (loginCodeSending ? '发送中...' : '获取验证码') }}
              </button>
            </div>
          </template>
          
          <button @click="handleLogin" :disabled="authLoading" class="submit-btn">
            {{ authLoading ? '登录中...' : '登录' }}
          </button>
          <p class="switch-text">
            <a @click="showLoginModal = false; showResetModal = true">忘记密码？</a>
            <span style="margin: 0 8px;">|</span>
            没有账号？<a @click="showLoginModal = false; openRegisterModal()">立即注册</a>
          </p>
        </div>
      </div>
    </Transition>

    <!-- 注册弹窗 -->
    <Transition name="modal">
      <div v-if="showRegisterModal" class="modal-overlay" @click="showRegisterModal = false">
        <div class="modal auth-modal" @click.stop>
          <button class="modal-close" @click="showRegisterModal = false">×</button>
          <h2>🚀 创建账号</h2>
          <p class="modal-subtitle">注册后即可参与秒杀活动</p>
          
          <!-- 用户名 + 随机按钮 -->
          <div class="input-with-btn">
            <input v-model="registerForm.username" type="text" placeholder="用户名（至少3位）" />
            <button type="button" class="random-btn" @click="generateRandomUsername" title="随机生成">🎲</button>
          </div>
          
          <!-- 密码 + 验证提示 -->
          <div class="password-field">
            <input v-model="registerForm.password" type="password" placeholder="密码" @input="validatePassword" />
            <div class="password-rules">
              <span :class="{ valid: pwdRules.length }">✓ 至少6位</span>
              <span :class="{ valid: pwdRules.hasLetter }">✓ 包含字母</span>
              <span :class="{ valid: pwdRules.hasNumber }">✓ 包含数字</span>
            </div>
          </div>
          
          <input v-model="registerForm.email" type="email" placeholder="邮箱地址" />
          <div class="code-row">
            <input v-model="registerForm.code" type="text" placeholder="验证码" maxlength="6" />
            <button 
              @click="handleSendCode" 
              :disabled="codeSending || codeCountdown > 0"
              class="send-code-btn"
            >
              {{ codeCountdown > 0 ? `${codeCountdown}s` : (codeSending ? '发送中...' : '获取验证码') }}
            </button>
          </div>
          
          <!-- 昵称 + 随机按钮 -->
          <div class="input-with-btn">
            <input v-model="registerForm.nickname" type="text" placeholder="昵称" />
            <button type="button" class="random-btn" @click="generateRandomNickname" title="随机生成">🎲</button>
          </div>
          
          <button @click="handleRegister" :disabled="authLoading || !isPasswordValid" class="submit-btn">
            {{ authLoading ? '注册中...' : '注册' }}
          </button>
          <p class="switch-text">
            已有账号？<a @click="showRegisterModal = false; showLoginModal = true">去登录</a>
          </p>
        </div>
      </div>
    </Transition>

    <!-- 忘记密码弹窗 -->
    <Transition name="modal">
      <div v-if="showResetModal" class="modal-overlay" @click="showResetModal = false">
        <div class="modal auth-modal" @click.stop>
          <button class="modal-close" @click="showResetModal = false">×</button>
          <h2>🔐 重置密码</h2>
          <p class="modal-subtitle">{{ resetStep === 1 ? '输入邮箱获取验证码' : '设置新密码' }}</p>
          
          <!-- 步骤1：输入邮箱和验证码 -->
          <template v-if="resetStep === 1">
            <input v-model="resetForm.email" type="email" placeholder="邮箱地址" />
            <div class="code-row">
              <input v-model="resetForm.code" type="text" placeholder="验证码" maxlength="6" />
              <button 
                @click="handleSendResetCode" 
                :disabled="resetCodeSending || resetCodeCountdown > 0"
                class="send-code-btn"
              >
                {{ resetCodeCountdown > 0 ? `${resetCodeCountdown}s` : (resetCodeSending ? '发送中...' : '获取验证码') }}
              </button>
            </div>
            <button @click="resetStep = 2" :disabled="!resetForm.email || !resetForm.code" class="submit-btn">
              下一步
            </button>
          </template>
          
          <!-- 步骤2：设置新密码 -->
          <template v-else>
            <div class="password-field">
              <input v-model="resetForm.password" type="password" placeholder="新密码" @input="validateResetPassword" />
              <div class="password-rules">
                <span :class="{ valid: resetPwdRules.length }">✓ 至少6位</span>
                <span :class="{ valid: resetPwdRules.hasLetter }">✓ 包含字母</span>
                <span :class="{ valid: resetPwdRules.hasNumber }">✓ 包含数字</span>
              </div>
            </div>
            <input v-model="resetForm.confirmPassword" type="password" placeholder="确认新密码" />
            <button @click="handleResetPassword" :disabled="authLoading || !isResetPasswordValid" class="submit-btn">
              {{ authLoading ? '重置中...' : '重置密码' }}
            </button>
            <p class="switch-text">
              <a @click="resetStep = 1">← 返回上一步</a>
            </p>
          </template>
          
          <p v-if="resetStep === 1" class="switch-text">
            想起密码了？<a @click="showResetModal = false; showLoginModal = true">去登录</a>
          </p>
        </div>
      </div>
    </Transition>

    <!-- 秒杀结果弹窗 -->
    <Transition name="modal">
      <div v-if="showResult" class="modal-overlay" @click="showResult = false">
        <div class="modal result-modal" :class="resultType" @click.stop>
          <div class="result-icon">{{ resultIcon }}</div>
          <h2>{{ resultTitle }}</h2>
          <p>{{ resultMessage }}</p>
          <p v-if="orderId" class="order-id">订单号：{{ orderId }}</p>
          <button @click="showResult = false" class="submit-btn">确定</button>
        </div>
      </div>
    </Transition>

    <!-- 加载状态 -->
    <Transition name="fade">
      <div v-if="loading" class="loading">
        <div class="loading-spinner"></div>
        <span>秒杀中...</span>
      </div>
    </Transition>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, reactive, watch } from 'vue'
import { login, loginByCode, register, doSeckill, getGoodsList, getSeckillRecords, sendVerifyCode, sendLoginCode, sendResetCode, resetPassword, getMyOrders, getOrderDetail, payOrder as apiPayOrder, cancelOrder as apiCancelOrder } from './api/seckill'

// 用户状态
const user = ref(null)

// 秒杀记录
const seckillRecords = ref([])
const isLoggedIn = computed(() => !!user.value)

// Toast 通知
const toast = ref({ show: false, message: '', type: 'info' })
const toastIcon = computed(() => {
  const icons = { success: '✓', error: '✕', warning: '!', info: 'ℹ' }
  return icons[toast.value.type] || 'ℹ'
})

const showToast = (message, type = 'info', duration = 3000) => {
  toast.value = { show: true, message, type }
  setTimeout(() => { toast.value.show = false }, duration)
}

// 弹窗控制
const showLoginModal = ref(false)
const showRegisterModal = ref(false)
const showResetModal = ref(false)
const showResult = ref(false)
const showPayModal = ref(false)
const authLoading = ref(false)

// 支付相关
const payOrder = ref(null)
const payCountdown = ref(0)
const paying = ref(false)
const cancelling = ref(false)
let countdownTimer = null

// 订单筛选和分页
const orderStatus = ref(-1) // -1 全部, 0 待支付, 1 已支付, 2 已取消
const orderFilters = [
  { label: '全部', value: -1 },
  { label: '待支付', value: 0 },
  { label: '已支付', value: 1 },
  { label: '已取消', value: 2 }
]
const orderPagination = ref({ page: 1, pageSize: 10, total: 0, totalPages: 0 })

// 登录方式：password / code
const loginMode = ref('password')

// 表单数据
const loginForm = ref({ email: '', password: '', code: '' })
const registerForm = ref({ username: '', password: '', email: '', code: '', nickname: '' })
const resetForm = ref({ email: '', code: '', password: '', confirmPassword: '' })
const codeSending = ref(false)
const codeCountdown = ref(0)
const loginCodeSending = ref(false)
const loginCodeCountdown = ref(0)
const resetCodeSending = ref(false)
const resetCodeCountdown = ref(0)
const resetStep = ref(1) // 1: 输入邮箱验证码, 2: 设置新密码

// 密码验证规则
const pwdRules = reactive({ length: false, hasLetter: false, hasNumber: false })
const isPasswordValid = computed(() => pwdRules.length && pwdRules.hasLetter && pwdRules.hasNumber)

const validatePassword = () => {
  const pwd = registerForm.value.password
  pwdRules.length = pwd.length >= 6
  pwdRules.hasLetter = /[a-zA-Z]/.test(pwd)
  pwdRules.hasNumber = /[0-9]/.test(pwd)
}

// 重置密码验证规则
const resetPwdRules = reactive({ length: false, hasLetter: false, hasNumber: false })
const isResetPasswordValid = computed(() => {
  return resetPwdRules.length && resetPwdRules.hasLetter && resetPwdRules.hasNumber && 
         resetForm.value.password === resetForm.value.confirmPassword
})

const validateResetPassword = () => {
  const pwd = resetForm.value.password
  resetPwdRules.length = pwd.length >= 6
  resetPwdRules.hasLetter = /[a-zA-Z]/.test(pwd)
  resetPwdRules.hasNumber = /[0-9]/.test(pwd)
}

// 随机昵称词库
const adjectives = ['快乐的', '勇敢的', '聪明的', '可爱的', '神秘的', '闪亮的', '飞翔的', '奔跑的', '微笑的', '幸运的']
const nouns = ['小猫', '小狗', '熊猫', '兔子', '老虎', '狮子', '海豚', '蝴蝶', '星星', '月亮', '太阳', '彩虹']

// 生成随机昵称
const generateRandomNickname = () => {
  const adj = adjectives[Math.floor(Math.random() * adjectives.length)]
  const noun = nouns[Math.floor(Math.random() * nouns.length)]
  const num = Math.floor(Math.random() * 1000)
  registerForm.value.nickname = `${adj}${noun}${num}`
}

// 生成随机用户名
const generateRandomUsername = () => {
  const chars = 'abcdefghijklmnopqrstuvwxyz'
  let username = ''
  for (let i = 0; i < 4; i++) {
    username += chars[Math.floor(Math.random() * chars.length)]
  }
  username += Math.floor(Math.random() * 10000)
  registerForm.value.username = username
}

// 打开注册弹窗时预填随机昵称
const openRegisterModal = () => {
  showRegisterModal.value = true
  if (!registerForm.value.nickname) {
    generateRandomNickname()
  }
}

// 秒杀相关
const loading = ref(false)
const resultType = ref('success')
const resultTitle = ref('')
const resultMessage = ref('')
const orderId = ref('')
const goodsList = ref([])
const myOrders = ref([]) // 我的订单列表

const resultIcon = computed(() => {
  const icons = { success: '🎉', warning: '😢', error: '❌' }
  return icons[resultType.value] || '📢'
})

// 登录（支持密码和验证码两种方式）
const handleLogin = async () => {
  if (!loginForm.value.email) {
    showToast('请输入邮箱地址', 'warning')
    return
  }
  
  if (loginMode.value === 'password') {
    if (!loginForm.value.password) {
      showToast('请输入密码', 'warning')
      return
    }
  } else {
    if (!loginForm.value.code) {
      showToast('请输入验证码', 'warning')
      return
    }
  }
  
  authLoading.value = true
  try {
    let res
    if (loginMode.value === 'password') {
      res = await login(loginForm.value.email, loginForm.value.password)
    } else {
      res = await loginByCode(loginForm.value.email, loginForm.value.code)
    }
    
    if (res.data.code === 0) {
      const { token, user_id, username, nickname } = res.data.data
      localStorage.setItem('token', token)
      localStorage.setItem('user', JSON.stringify({ user_id, username, nickname }))
      user.value = { user_id, username, nickname }
      showLoginModal.value = false
      loginForm.value = { email: '', password: '', code: '' }
      showToast(`欢迎回来，${nickname || username}！`, 'success')
    } else {
      showToast(res.data.message, 'error')
    }
  } catch (err) {
    showToast(err.response?.data?.message || '登录失败', 'error')
  } finally {
    authLoading.value = false
  }
}

// 发送登录验证码
const handleSendLoginCode = async () => {
  if (!loginForm.value.email) {
    showToast('请输入邮箱地址', 'warning')
    return
  }
  
  loginCodeSending.value = true
  try {
    const res = await sendLoginCode(loginForm.value.email)
    if (res.data.code === 0) {
      showToast('验证码已发送，请查收邮件', 'success')
      loginCodeCountdown.value = 60
      const timer = setInterval(() => {
        loginCodeCountdown.value--
        if (loginCodeCountdown.value <= 0) clearInterval(timer)
      }, 1000)
    } else {
      showToast(res.data.message, 'error')
    }
  } catch (err) {
    showToast(err.response?.data?.message || '发送失败', 'error')
  } finally {
    loginCodeSending.value = false
  }
}

// 发送注册验证码
const handleSendCode = async () => {
  if (!registerForm.value.email) {
    showToast('请输入邮箱地址', 'warning')
    return
  }
  
  codeSending.value = true
  try {
    const res = await sendVerifyCode(registerForm.value.email)
    if (res.data.code === 0) {
      showToast('验证码已发送，请查收邮件', 'success')
      codeCountdown.value = 60
      const timer = setInterval(() => {
        codeCountdown.value--
        if (codeCountdown.value <= 0) clearInterval(timer)
      }, 1000)
    } else {
      showToast(res.data.message, 'error')
    }
  } catch (err) {
    showToast(err.response?.data?.message || '发送失败', 'error')
  } finally {
    codeSending.value = false
  }
}

// 注册
const handleRegister = async () => {
  if (!registerForm.value.username || !registerForm.value.password) {
    showToast('请输入用户名和密码', 'warning')
    return
  }
  if (!isPasswordValid.value) {
    showToast('密码需要至少6位，包含字母和数字', 'warning')
    return
  }
  if (!registerForm.value.email || !registerForm.value.code) {
    showToast('请输入邮箱和验证码', 'warning')
    return
  }
  if (!registerForm.value.nickname) {
    showToast('请输入昵称', 'warning')
    return
  }
  
  authLoading.value = true
  try {
    const res = await register(
      registerForm.value.username,
      registerForm.value.password,
      registerForm.value.email,
      registerForm.value.code,
      registerForm.value.nickname
    )
    if (res.data.code === 0) {
      showToast('注册成功，请登录', 'success')
      showRegisterModal.value = false
      showLoginModal.value = true
      // 自动填充邮箱到登录表单
      loginForm.value.email = registerForm.value.email
      registerForm.value = { username: '', password: '', email: '', code: '', nickname: '' }
      pwdRules.length = false
      pwdRules.hasLetter = false
      pwdRules.hasNumber = false
    } else {
      showToast(res.data.message, 'error')
    }
  } catch (err) {
    showToast(err.response?.data?.message || '注册失败', 'error')
  } finally {
    authLoading.value = false
  }
}

// 退出登录
const handleLogout = () => {
  localStorage.removeItem('token')
  localStorage.removeItem('user')
  user.value = null
  showToast('已退出登录', 'info')
}

// 发送重置密码验证码
const handleSendResetCode = async () => {
  if (!resetForm.value.email) {
    showToast('请输入邮箱地址', 'warning')
    return
  }
  
  resetCodeSending.value = true
  try {
    const res = await sendResetCode(resetForm.value.email)
    if (res.data.code === 0) {
      showToast('验证码已发送，请查收邮件', 'success')
      resetCodeCountdown.value = 60
      const timer = setInterval(() => {
        resetCodeCountdown.value--
        if (resetCodeCountdown.value <= 0) clearInterval(timer)
      }, 1000)
    } else {
      showToast(res.data.message, 'error')
    }
  } catch (err) {
    showToast(err.response?.data?.message || '发送失败', 'error')
  } finally {
    resetCodeSending.value = false
  }
}

// 重置密码
const handleResetPassword = async () => {
  if (!resetForm.value.email || !resetForm.value.code) {
    showToast('请输入邮箱和验证码', 'warning')
    return
  }
  if (!isResetPasswordValid.value) {
    if (resetForm.value.password !== resetForm.value.confirmPassword) {
      showToast('两次输入的密码不一致', 'warning')
    } else {
      showToast('密码需要至少6位，包含字母和数字', 'warning')
    }
    return
  }
  
  authLoading.value = true
  try {
    const res = await resetPassword(resetForm.value.email, resetForm.value.code, resetForm.value.password)
    if (res.data.code === 0) {
      showToast('密码重置成功，请登录', 'success')
      showResetModal.value = false
      showLoginModal.value = true
      loginForm.value.email = resetForm.value.email
      resetForm.value = { email: '', code: '', password: '', confirmPassword: '' }
      resetStep.value = 1
      resetPwdRules.length = false
      resetPwdRules.hasLetter = false
      resetPwdRules.hasNumber = false
    } else {
      showToast(res.data.message, 'error')
    }
  } catch (err) {
    showToast(err.response?.data?.message || '重置失败', 'error')
  } finally {
    authLoading.value = false
  }
}

// 执行秒杀
const handleSeckill = async (goodsId) => {
  if (!isLoggedIn.value) {
    showLoginModal.value = true
    return
  }

  loading.value = true
  
  try {
    const res = await doSeckill(goodsId)
    const { code, message, data } = res.data

    if (code === 0) {
      resultType.value = 'success'
      resultTitle.value = '秒杀成功！'
      resultMessage.value = message
      orderId.value = data?.order_id || ''
      loadGoods()
      loadSeckillRecords() // 刷新秒杀记录
      
      // 乐观更新：立即在本地添加订单显示
      const goods = goodsList.value.find(g => g.id === goodsId)
      const newOrder = {
        order_id: data?.order_id || '',
        goods_id: goodsId,
        goods_name: goods?.name || '商品',
        status: 0, // 待支付
        created_at: new Date().toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-')
      }
      myOrders.value = [newOrder, ...myOrders.value]
      
      // 后台同步刷新（确保数据一致）
      setTimeout(() => loadMyOrders(), 2000)
    } else if (code === 1) {
      resultType.value = 'warning'
      resultTitle.value = '很遗憾'
      resultMessage.value = message
      orderId.value = ''
    } else if (code === 2) {
      resultType.value = 'warning'
      resultTitle.value = '提示'
      resultMessage.value = message
      orderId.value = ''
    } else {
      resultType.value = 'error'
      resultTitle.value = '系统繁忙'
      resultMessage.value = message
      orderId.value = ''
    }
  } catch (err) {
    if (err.response?.status === 401) {
      showLoginModal.value = true
      return
    }
    resultType.value = 'error'
    resultTitle.value = '请求失败'
    resultMessage.value = err.response?.data?.message || '网络错误'
    orderId.value = ''
  } finally {
    loading.value = false
    showResult.value = true
  }
}

// 加载商品列表
const loadGoods = async () => {
  try {
    const res = await getGoodsList()
    if (res.data.code === 0) {
      goodsList.value = res.data.data
    }
  } catch (err) {
    console.error('获取商品列表失败:', err)
  }
}

// 加载秒杀记录
const loadSeckillRecords = async () => {
  try {
    const res = await getSeckillRecords()
    if (res.data.code === 0) {
      seckillRecords.value = res.data.data || []
    }
  } catch (err) {
    console.error('获取秒杀记录失败:', err)
  }
}

// 加载我的订单（支持筛选和分页）
const loadMyOrders = async () => {
  if (!isLoggedIn.value) {
    myOrders.value = []
    return
  }
  try {
    const params = {
      page: orderPagination.value.page,
      page_size: orderPagination.value.pageSize
    }
    if (orderStatus.value !== -1) {
      params.status = orderStatus.value
    }
    const res = await getMyOrders(params)
    if (res.data.code === 0) {
      const data = res.data.data
      myOrders.value = data.list || []
      orderPagination.value.total = data.total
      orderPagination.value.totalPages = data.total_pages
    }
  } catch (err) {
    console.error('获取订单列表失败:', err)
  }
}

// 切换订单状态筛选
const changeOrderStatus = (status) => {
  orderStatus.value = status
  orderPagination.value.page = 1
  loadMyOrders()
}

// 切换订单页码
const changeOrderPage = (page) => {
  orderPagination.value.page = page
  loadMyOrders()
}

// 检查订单是否超时（创建时间超过 1 分钟）
const isOrderTimeout = (createdAt) => {
  if (!createdAt) return false
  const created = new Date(createdAt.replace(/-/g, '/')) // 兼容 Safari
  const now = new Date()
  return (now - created) > 60 * 1000 // 超过 60 秒
}

// 获取订单的实际状态（考虑超时）
const getActualStatus = (order) => {
  if (order.status === 0 && isOrderTimeout(order.created_at)) {
    return 2 // 超时视为已取消
  }
  return order.status
}

// 获取订单状态文本
const getStatusText = (order) => {
  const status = getActualStatus(order)
  const statusMap = { 0: '待支付', 1: '已支付', 2: '已取消' }
  return statusMap[status] || '未知'
}

// 获取订单状态样式
const getStatusClass = (order) => {
  const status = getActualStatus(order)
  const classMap = { 0: 'pending', 1: 'paid', 2: 'cancelled' }
  return classMap[status] || ''
}

// 检查订单是否可点击（待支付且未超时）
const isOrderClickable = (order) => {
  return order.status === 0 && !isOrderTimeout(order.created_at)
}

// 格式化倒计时
const formatCountdown = (seconds) => {
  if (seconds <= 0) return '00:00'
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
}

// 打开支付弹窗
const openPayModal = async (orderId) => {
  try {
    const res = await getOrderDetail(orderId)
    if (res.data.code === 0) {
      payOrder.value = res.data.data
      payCountdown.value = res.data.data.remain_seconds || 0
      showPayModal.value = true
      
      // 启动倒计时
      if (payOrder.value.status === 0 && payCountdown.value > 0) {
        startCountdown()
      }
    } else if (res.data.code === 404) {
      // 订单还未写入数据库，提示用户稍等
      showToast('订单处理中，请稍后再试', 'warning')
      setTimeout(() => loadMyOrders(), 1000)
    } else {
      showToast(res.data.message, 'error')
    }
  } catch (err) {
    if (err.response?.status === 404) {
      showToast('订单处理中，请稍后再试', 'warning')
      setTimeout(() => loadMyOrders(), 1000)
    } else {
      showToast('获取订单详情失败', 'error')
    }
  }
}

// 关闭支付弹窗
const closePayModal = () => {
  showPayModal.value = false
  stopCountdown()
  loadMyOrders() // 刷新订单列表
  loadGoods() // 刷新库存
}

// 启动倒计时
const startCountdown = () => {
  stopCountdown()
  countdownTimer = setInterval(() => {
    if (payCountdown.value > 0) {
      payCountdown.value--
    } else {
      stopCountdown()
      // 超时自动取消
      if (payOrder.value) {
        payOrder.value.status = 2
      }
      showToast('订单已超时取消', 'warning')
    }
  }, 1000)
}

// 停止倒计时
const stopCountdown = () => {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
}

// 支付订单
const handlePay = async () => {
  if (!payOrder.value) return
  
  paying.value = true
  try {
    const res = await apiPayOrder(payOrder.value.order_id)
    if (res.data.code === 0) {
      showToast('支付成功！', 'success')
      payOrder.value.status = 1
      stopCountdown()
      // 同步更新订单列表中的状态
      const orderIndex = myOrders.value.findIndex(o => o.order_id === payOrder.value.order_id)
      if (orderIndex !== -1) {
        myOrders.value[orderIndex].status = 1
      }
    } else {
      showToast(res.data.message, 'error')
      // 如果订单已取消，更新状态
      if (res.data.message.includes('取消')) {
        payOrder.value.status = 2
        // 同步更新订单列表
        const orderIndex = myOrders.value.findIndex(o => o.order_id === payOrder.value.order_id)
        if (orderIndex !== -1) {
          myOrders.value[orderIndex].status = 2
        }
      }
    }
  } catch (err) {
    showToast(err.response?.data?.message || '支付失败', 'error')
  } finally {
    paying.value = false
  }
}

// 取消订单
const handleCancel = async () => {
  if (!payOrder.value) return
  
  cancelling.value = true
  try {
    const res = await apiCancelOrder(payOrder.value.order_id)
    if (res.data.code === 0) {
      showToast('订单已取消', 'success')
      payOrder.value.status = 2
      stopCountdown()
      // 同步更新订单列表中的状态，避免需要刷新才能看到最新状态
      const orderIndex = myOrders.value.findIndex(o => o.order_id === payOrder.value.order_id)
      if (orderIndex !== -1) {
        myOrders.value[orderIndex].status = 2
      }
    } else {
      showToast(res.data.message, 'error')
    }
  } catch (err) {
    showToast(err.response?.data?.message || '取消失败', 'error')
  } finally {
    cancelling.value = false
  }
}

// 监听登录状态变化，自动加载订单
watch(isLoggedIn, (newVal) => {
  if (newVal) {
    loadMyOrders()
  } else {
    myOrders.value = []
  }
})

// 订单超时检查定时器
let orderCheckTimer = null

// 检查并处理超时订单（静默调用后端取消接口）
const checkTimeoutOrders = async () => {
  for (const order of myOrders.value) {
    if (order.status === 0 && isOrderTimeout(order.created_at)) {
      // 静默调用后端取消接口，恢复库存
      try {
        await apiCancelOrder(order.order_id)
        order.status = 2 // 更新本地状态
      } catch (err) {
        // 忽略错误，可能已被其他地方取消
      }
    }
  }
}

// 启动订单超时检查定时器
const startOrderCheckTimer = () => {
  stopOrderCheckTimer()
  // 每秒检查一次，触发视图更新
  orderCheckTimer = setInterval(() => {
    // 触发响应式更新（强制重新计算状态）
    myOrders.value = [...myOrders.value]
    // 静默处理超时订单
    checkTimeoutOrders()
  }, 1000)
}

// 停止订单超时检查定时器
const stopOrderCheckTimer = () => {
  if (orderCheckTimer) {
    clearInterval(orderCheckTimer)
    orderCheckTimer = null
  }
}

// 初始化
onMounted(() => {
  const savedUser = localStorage.getItem('user')
  if (savedUser) {
    user.value = JSON.parse(savedUser)
  }
  loadGoods()
  loadSeckillRecords()
  loadMyOrders()
  startOrderCheckTimer() // 启动订单超时检查
})

// 清理定时器
onUnmounted(() => {
  stopCountdown()
  stopOrderCheckTimer()
})
</script>

<style>
* { margin: 0; padding: 0; box-sizing: border-box; }

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  background: #f0f2f5;
  min-height: 100vh;
}

.container { max-width: 800px; margin: 0 auto; padding: 40px 20px; }
h1 { text-align: center; color: #333; margin-bottom: 20px; font-size: 2.5rem; }

/* Toast */
.toast {
  position: fixed; top: 30px; left: 50%; transform: translateX(-50%);
  padding: 14px 28px; border-radius: 12px; display: flex; align-items: center;
  gap: 12px; font-size: 15px; font-weight: 500; z-index: 2000;
  box-shadow: 0 8px 30px rgba(0,0,0,0.2); backdrop-filter: blur(10px);
}
.toast-icon { width: 24px; height: 24px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: 14px; font-weight: bold; }
.toast.success { background: rgba(39, 174, 96, 0.95); color: white; }
.toast.error { background: rgba(231, 76, 60, 0.95); color: white; }
.toast.warning { background: rgba(243, 156, 18, 0.95); color: white; }
.toast.info { background: rgba(52, 152, 219, 0.95); color: white; }
.toast-icon { background: rgba(255,255,255,0.2); }
.toast-enter-active, .toast-leave-active { transition: all 0.3s ease; }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateX(-50%) translateY(-20px); }

/* 用户栏 */
.user-bar {
  background: rgba(255,255,255,0.95); padding: 15px 20px; border-radius: 12px;
  margin-bottom: 30px; display: flex; align-items: center; justify-content: flex-end;
  gap: 15px; backdrop-filter: blur(10px); box-shadow: 0 4px 20px rgba(0,0,0,0.1);
}
.user-bar span { margin-right: auto; font-weight: 600; color: #333; }
.login-btn, .register-btn, .logout-btn { padding: 10px 24px; border: none; border-radius: 8px; cursor: pointer; font-size: 14px; font-weight: 500; transition: all 0.2s; }
.login-btn { background: #f39c12; color: white; }
.login-btn:hover { transform: scale(1.05); box-shadow: 0 4px 15px rgba(243,156,18,0.4); }
.register-btn { background: #f0f0f0; color: #333; }
.register-btn:hover { transform: scale(1.05); }
.logout-btn { background: #e74c3c; color: white; }
.logout-btn:hover { transform: scale(1.05); box-shadow: 0 4px 15px rgba(231,76,60,0.4); }

/* 秒杀记录滚动展示 */
.seckill-records {
  background: rgba(255,255,255,0.95); border-radius: 12px; padding: 15px 20px;
  margin-bottom: 25px; box-shadow: 0 4px 20px rgba(0,0,0,0.1); overflow: hidden;
}
.records-title { font-size: 14px; font-weight: 600; color: #f39c12; margin-bottom: 10px; }
.records-scroll { overflow: hidden; position: relative; }
.records-track {
  display: flex; gap: 30px; white-space: nowrap;
  animation: scroll-left 20s linear infinite;
}
.record-item {
  font-size: 13px; color: #666; padding: 6px 12px;
  background: #f8f9fa; border-radius: 20px; flex-shrink: 0;
}
@keyframes scroll-left {
  0% { transform: translateX(0); }
  100% { transform: translateX(-50%); }
}
.records-scroll:hover .records-track { animation-play-state: paused; }

/* 商品列表 */
.goods-list { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 20px; }
.goods-card { background: rgba(255,255,255,0.95); border-radius: 16px; padding: 30px; text-align: center; box-shadow: 0 10px 40px rgba(0,0,0,0.1); transition: all 0.2s; backdrop-filter: blur(10px); }
.goods-card:hover { transform: scale(1.03); box-shadow: 0 20px 50px rgba(0,0,0,0.15); }
.goods-card.sold-out { opacity: 0.6; }
.goods-card h3 { color: #333; margin-bottom: 15px; font-size: 1.3rem; }
.stock { color: #e74c3c; font-size: 1.1rem; font-weight: 600; margin-bottom: 20px; }
.seckill-btn { width: 100%; padding: 15px 30px; font-size: 18px; font-weight: 600; color: white; background: #f39c12; border: none; border-radius: 10px; cursor: pointer; transition: all 0.2s; }
.seckill-btn:hover:not(:disabled) { transform: scale(1.05); box-shadow: 0 8px 25px rgba(243,156,18,0.4); }
.seckill-btn:disabled { background: #bdc3c7; cursor: not-allowed; }

/* 弹窗 */
.modal-overlay { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.6); display: flex; align-items: center; justify-content: center; z-index: 1000; backdrop-filter: blur(4px); }
.modal { background: white; padding: 40px; border-radius: 20px; text-align: center; max-width: 420px; width: 90%; position: relative; box-shadow: 0 25px 60px rgba(0,0,0,0.3); }
.modal-close { position: absolute; top: 15px; right: 20px; background: none; border: none; font-size: 28px; color: #999; cursor: pointer; transition: color 0.2s; line-height: 1; }
.modal-close:hover { color: #333; }
.modal h2 { margin-bottom: 8px; font-size: 1.6rem; color: #333; }
.modal-subtitle { color: #888; margin-bottom: 25px; font-size: 14px; }

/* 登录方式切换 */
.login-tabs { display: flex; gap: 0; margin-bottom: 20px; border-radius: 10px; overflow: hidden; border: 2px solid #eee; }
.login-tabs button { flex: 1; padding: 12px; border: none; background: #f8f9fa; color: #666; font-size: 14px; font-weight: 500; cursor: pointer; transition: all 0.2s; }
.login-tabs button.active { background: #f39c12; color: white; }
.login-tabs button:hover:not(.active) { background: #eee; }

.auth-modal input { width: 100%; padding: 14px 18px; margin-bottom: 15px; border: 2px solid #eee; border-radius: 10px; font-size: 15px; transition: all 0.2s; }
.auth-modal input:focus { outline: none; border-color: #f39c12; box-shadow: 0 0 0 4px rgba(243,156,18,0.1); }

.input-with-btn { display: flex; gap: 8px; margin-bottom: 15px; }
.input-with-btn input { flex: 1; margin-bottom: 0; }
.random-btn { width: 48px; height: 48px; border: 2px solid #eee; border-radius: 10px; background: #f8f9fa; font-size: 20px; cursor: pointer; transition: all 0.2s; display: flex; align-items: center; justify-content: center; }
.random-btn:hover { background: #f39c12; border-color: #f39c12; transform: scale(1.1); }

.password-field { margin-bottom: 15px; }
.password-field input { margin-bottom: 8px; }
.password-rules { display: flex; gap: 12px; justify-content: center; flex-wrap: wrap; }
.password-rules span { font-size: 12px; color: #ccc; transition: color 0.2s; }
.password-rules span.valid { color: #27ae60; }

.submit-btn { width: 100%; padding: 14px; background: #f39c12; color: white; border: none; border-radius: 10px; font-size: 16px; font-weight: 600; cursor: pointer; margin-top: 10px; transition: all 0.2s; }
.submit-btn:hover:not(:disabled) { transform: scale(1.02); box-shadow: 0 8px 25px rgba(243,156,18,0.4); }
.submit-btn:disabled { background: #bdc3c7; cursor: not-allowed; }

.code-row { display: flex; gap: 10px; margin-bottom: 15px; }
.code-row input { flex: 1; margin-bottom: 0; }
.send-code-btn { padding: 14px 18px !important; font-size: 14px !important; white-space: nowrap; background: #f39c12; color: white; border: none; border-radius: 10px; cursor: pointer; transition: all 0.2s; }
.send-code-btn:hover:not(:disabled) { transform: scale(1.05); }
.send-code-btn:disabled { background: #bdc3c7; cursor: not-allowed; }

.switch-text { margin-top: 25px; color: #888; font-size: 14px; }
.switch-text a { color: #e67e22; cursor: pointer; font-weight: 500; }
.switch-text a:hover { text-decoration: underline; }

/* 结果弹窗 */
.result-modal .result-icon { font-size: 64px; margin-bottom: 15px; }
.result-modal.success h2 { color: #27ae60; }
.result-modal.warning h2 { color: #f39c12; }
.result-modal.error h2 { color: #e74c3c; }
.result-modal p { color: #666; margin-bottom: 10px; line-height: 1.6; }
.order-id { font-family: 'SF Mono', Monaco, monospace; background: #f8f9fa; padding: 12px; border-radius: 8px; font-size: 12px; word-break: break-all; color: #555; margin-top: 15px; }

/* 动画 */
.modal-enter-active, .modal-leave-active { transition: all 0.3s ease; }
.modal-enter-from, .modal-leave-to { opacity: 0; }
.modal-enter-from .modal, .modal-leave-to .modal { transform: scale(0.9); }

/* 加载 */
.loading { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.7); display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 20px; z-index: 1001; backdrop-filter: blur(4px); }
.loading-spinner { width: 50px; height: 50px; border: 4px solid rgba(255,255,255,0.3); border-top-color: white; border-radius: 50%; animation: spin 0.8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.loading span { color: white; font-size: 18px; font-weight: 500; }
.fade-enter-active, .fade-leave-active { transition: opacity 0.3s ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }

/* 我的订单 */
.my-orders { margin-top: 40px; }
.orders-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; flex-wrap: wrap; gap: 15px; }
.orders-header h2 { color: #333; font-size: 1.5rem; margin: 0; }
.order-filters { display: flex; gap: 8px; }
.filter-btn { padding: 8px 16px; border: 2px solid #eee; border-radius: 20px; background: white; color: #666; font-size: 13px; cursor: pointer; transition: all 0.2s; }
.filter-btn:hover { border-color: #f39c12; color: #f39c12; }
.filter-btn.active { background: #f39c12; border-color: #f39c12; color: white; }
.orders-list { display: flex; flex-direction: column; gap: 15px; }
.order-card { background: rgba(255,255,255,0.95); border-radius: 12px; padding: 20px; backdrop-filter: blur(10px); box-shadow: 0 4px 20px rgba(0,0,0,0.1); }
.order-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
.order-goods { font-weight: 600; color: #333; font-size: 1.1rem; }
.order-status { padding: 4px 12px; border-radius: 20px; font-size: 12px; font-weight: 500; }
.order-status.pending { background: #fff3cd; color: #856404; }
.order-status.paid { background: #d4edda; color: #155724; }
.order-status.cancelled { background: #f8d7da; color: #721c24; }
.order-info { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 10px; }
.order-id { font-family: 'SF Mono', Monaco, monospace; font-size: 12px; color: #666; background: #f5f5f5; padding: 4px 8px; border-radius: 4px; }
.order-time { font-size: 13px; color: #888; }
.order-card.clickable { cursor: pointer; transition: all 0.2s; }
.order-card.clickable:hover { transform: scale(1.02); box-shadow: 0 8px 30px rgba(0,0,0,0.15); }
.order-tip { margin-top: 12px; color: #f39c12; font-size: 13px; font-weight: 500; }
.no-orders { text-align: center; padding: 40px; color: #999; background: rgba(255,255,255,0.95); border-radius: 12px; }
.pagination { display: flex; justify-content: center; align-items: center; gap: 15px; margin-top: 20px; }
.page-btn { padding: 8px 20px; border: 2px solid #eee; border-radius: 8px; background: white; color: #666; cursor: pointer; transition: all 0.2s; }
.page-btn:hover:not(:disabled) { border-color: #f39c12; color: #f39c12; }
.page-btn:disabled { opacity: 0.5; cursor: not-allowed; }
.page-info { font-size: 14px; color: #666; }

/* 支付弹窗 */
.pay-modal { max-width: 380px; }
.pay-info { margin: 20px 0; }
.pay-goods { font-size: 1.3rem; font-weight: 600; color: #333; margin-bottom: 10px; }
.pay-order-id { font-size: 12px; color: #888; margin-bottom: 20px; font-family: 'SF Mono', Monaco, monospace; }
.pay-countdown { background: #fff3cd; padding: 15px; border-radius: 10px; margin-bottom: 20px; }
.countdown-label { display: block; font-size: 13px; color: #856404; margin-bottom: 5px; }
.countdown-time { font-size: 2rem; font-weight: 700; color: #e74c3c; font-family: 'SF Mono', Monaco, monospace; }
.pay-success { background: #d4edda; padding: 20px; border-radius: 10px; margin-bottom: 20px; color: #155724; }
.pay-success .success-icon { font-size: 2rem; display: block; margin-bottom: 5px; }
.pay-cancelled { background: #f8d7da; padding: 20px; border-radius: 10px; margin-bottom: 20px; color: #721c24; }
.pay-cancelled .cancelled-icon { font-size: 2rem; display: block; margin-bottom: 5px; }
.pay-actions { display: flex; gap: 10px; }
.pay-actions .submit-btn { flex: 1; }
.pay-btn { background: #27ae60 !important; }
.pay-btn:hover:not(:disabled) { transform: scale(1.02); box-shadow: 0 8px 25px rgba(39,174,96,0.4) !important; }
.cancel-btn { background: #95a5a6 !important; }
.cancel-btn:hover:not(:disabled) { transform: scale(1.02); box-shadow: 0 8px 25px rgba(149,165,166,0.4) !important; }
</style>
