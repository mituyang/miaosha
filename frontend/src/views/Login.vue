<template>
  <div class="auth-page">
    <div class="auth-card">
      <div class="auth-copy">
        <span class="auth-kicker">Miaosha</span>
        <h1 class="auth-title">登录账号</h1>
        <p class="auth-subtitle">进入秒杀前台，查看当前上架商品、实时库存和订单状态。</p>
      </div>

      <div v-if="error" class="alert alert-error">{{ error }}</div>

      <form @submit.prevent="handleSubmit" class="auth-form">
        <div class="form-group">
          <label class="form-label" for="login-username">用户名</label>
          <input
            id="login-username"
            v-model="form.username"
            type="text"
            class="form-input"
            placeholder="请输入用户名"
            autocomplete="username"
          />
        </div>

        <div class="form-group">
          <label class="form-label" for="login-password">密码</label>
          <input
            id="login-password"
            v-model="form.password"
            type="password"
            class="form-input"
            placeholder="请输入密码"
            autocomplete="current-password"
          />
        </div>

        <div class="form-group">
          <label class="form-label" for="login-captcha">验证码</label>
          <div class="captcha-row">
            <input
              id="login-captcha"
              v-model="form.captchaCode"
              type="text"
              class="form-input"
              placeholder="请输入验证码"
              maxlength="4"
            />
            <button type="button" class="captcha-button" @click="fetchCaptcha" :disabled="captchaLoading">
              <img v-if="captchaImage" :src="captchaImage" alt="验证码" class="captcha-image" />
              <span v-else>{{ captchaLoading ? '加载中...' : '刷新验证码' }}</span>
            </button>
          </div>
        </div>

        <button type="submit" class="btn btn-primary btn-block" :disabled="loading">
          {{ loading ? '登录中...' : '登录' }}
        </button>
      </form>

      <div class="auth-footer">
        还没有账号？<router-link to="/register" class="link">立即注册</router-link>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { getLoginCaptcha, login } from '../api'

const router = useRouter()
const loading = ref(false)
const error = ref('')
const captchaLoading = ref(false)
const captchaId = ref('')
const captchaImage = ref('')
const form = reactive({ username: '', password: '', captchaCode: '' })

const fetchCaptcha = async () => {
  captchaLoading.value = true
  try {
    const res = await getLoginCaptcha()
    if (res.data.code === 0) {
      captchaId.value = res.data.data.captcha_id
      captchaImage.value = res.data.data.captcha_image
      form.captchaCode = ''
    } else {
      error.value = res.data.msg || '获取验证码失败'
    }
  } catch (e) {
    error.value = e.response?.data?.msg || '获取验证码失败'
  } finally {
    captchaLoading.value = false
  }
}

const handleSubmit = async () => {
  error.value = ''
  
  if (!form.username || !form.password || !form.captchaCode) {
    error.value = '请填写用户名、密码和验证码'
    return
  }

  if (!captchaId.value) {
    error.value = '验证码未加载，请刷新后重试'
    return
  }
  
  loading.value = true
  try {
    const res = await login(form.username, form.password, captchaId.value, form.captchaCode)
    if (res.data.code === 0) {
      localStorage.setItem('token', res.data.data.token)
      localStorage.setItem('username', form.username)
      router.push('/seckill')
    } else {
      error.value = res.data.msg
      await fetchCaptcha()
    }
  } catch (e) {
    error.value = e.response?.data?.msg || '登录失败'
    await fetchCaptcha()
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchCaptcha()
})
</script>

<style scoped>
.captcha-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 132px;
  gap: 12px;
  align-items: center;
}

.captcha-button {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 132px;
  height: 44px;
  border: 1px solid rgba(92, 68, 42, 0.18);
  border-radius: 12px;
  background: #f8f2ea;
  padding: 0;
  overflow: hidden;
  cursor: pointer;
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 600;
}

.captcha-button:disabled {
  cursor: wait;
}

.captcha-image {
  display: block;
  width: 132px;
  height: 44px;
  object-fit: contain;
}

@media (max-width: 420px) {
  .captcha-row {
    grid-template-columns: minmax(0, 1fr);
  }

  .captcha-button,
  .captcha-image {
    width: 100%;
  }
}
</style>
