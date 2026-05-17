<template>
  <div class="auth-page">
    <div class="auth-card">
      <div class="auth-copy">
        <span class="auth-kicker">Miaosha</span>
        <h1 class="auth-title">找回密码</h1>
        <p class="auth-subtitle">通过注册邮箱接收验证码，重新设置登录密码。</p>
      </div>

      <div v-if="error" class="alert alert-error">{{ error }}</div>
      <div v-if="success" class="alert alert-success">{{ success }}</div>

      <form @submit.prevent="handleSubmit" class="auth-form">
        <div class="form-group">
          <label class="form-label" for="forgot-email">邮箱</label>
          <input
            id="forgot-email"
            v-model.trim="form.email"
            type="email"
            class="form-input"
            placeholder="请输入注册邮箱"
            autocomplete="email"
          />
        </div>

        <div class="form-group">
          <label class="form-label" for="forgot-password">新密码</label>
          <input
            id="forgot-password"
            v-model="form.password"
            type="password"
            class="form-input"
            placeholder="至少6个字符"
            autocomplete="new-password"
          />
          <p :class="['form-helper', passwordHintClass]">{{ passwordHintText }}</p>
        </div>

        <div class="form-group">
          <label class="form-label" for="forgot-confirm-password">确认新密码</label>
          <input
            id="forgot-confirm-password"
            v-model="form.confirmPassword"
            type="password"
            class="form-input"
            placeholder="再次输入新密码"
            autocomplete="new-password"
          />
          <p v-if="confirmPasswordHintText" :class="['form-helper', confirmPasswordHintClass]">{{ confirmPasswordHintText }}</p>
        </div>

        <div class="form-group">
          <label class="form-label" for="forgot-email-code">邮箱验证码</label>
          <div class="email-code-row">
            <input
              id="forgot-email-code"
              v-model.trim="form.emailCode"
              type="text"
              class="form-input"
              placeholder="请输入邮箱验证码"
            />
            <button type="button" class="btn btn-secondary email-code-button" @click="openCaptchaModal" :disabled="sendingEmailCode || cooldownSeconds > 0">
              {{ emailCodeButtonText }}
            </button>
          </div>
          <p v-if="emailCodeExpiresText" class="form-helper">邮箱验证码{{ emailCodeExpiresText }}后过期</p>
        </div>

        <button type="submit" class="btn btn-primary btn-block" :disabled="loading">
          {{ loading ? '提交中...' : '重置密码' }}
        </button>
      </form>

      <div class="auth-footer">
        想起密码？<router-link to="/login" class="link">返回登录</router-link>
      </div>
    </div>

    <div v-if="captchaModalVisible" class="modal-backdrop" @click.self="closeCaptchaModal">
      <section class="captcha-modal" role="dialog" aria-modal="true" aria-labelledby="captcha-modal-title">
        <div class="modal-header">
          <h2 id="captcha-modal-title">安全验证</h2>
          <button type="button" class="modal-close" aria-label="关闭" @click="closeCaptchaModal" :disabled="sendingEmailCode">×</button>
        </div>

        <div class="form-group">
          <label class="form-label" for="forgot-captcha">验证码</label>
          <div class="captcha-row">
            <input
              id="forgot-captcha"
              ref="captchaInputRef"
              v-model.trim="form.captchaCode"
              type="text"
              class="form-input"
              placeholder="请输入验证码"
              maxlength="4"
              @keyup.enter="handleSendEmailCode"
            />
            <button type="button" class="captcha-button" @click="fetchCaptcha" :disabled="captchaLoading || sendingEmailCode">
              <img v-if="captchaImage" :src="captchaImage" alt="验证码" class="captcha-image" />
              <span v-else>{{ captchaLoading ? '加载中...' : '刷新验证码' }}</span>
            </button>
          </div>
        </div>

        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" @click="closeCaptchaModal" :disabled="sendingEmailCode">取消</button>
          <button type="button" class="btn btn-primary" @click="handleSendEmailCode" :disabled="sendingEmailCode || captchaLoading">
            {{ sendingEmailCode ? '发送中...' : '确认发送' }}
          </button>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { getLoginCaptcha, resetPassword, sendPasswordResetEmailCode } from '../api'

const router = useRouter()
const loading = ref(false)
const sendingEmailCode = ref(false)
const captchaLoading = ref(false)
const error = ref('')
const success = ref('')
const captchaId = ref('')
const captchaImage = ref('')
const captchaModalVisible = ref(false)
const captchaInputRef = ref(null)
const cooldownSeconds = ref(0)
const emailCodeExpiresSeconds = ref(0)
let cooldownTimer = null
const form = reactive({
  email: '',
  password: '',
  confirmPassword: '',
  captchaCode: '',
  emailCode: ''
})

const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

const emailCodeButtonText = computed(() => {
  if (sendingEmailCode.value) return '发送中...'
  if (cooldownSeconds.value > 0) return `${cooldownSeconds.value}s 后重发`
  return '发送验证码'
})

const emailCodeExpiresText = computed(() => {
  if (emailCodeExpiresSeconds.value <= 0) return ''
  return formatDurationText(emailCodeExpiresSeconds.value)
})

const passwordHintText = computed(() => {
  if (!form.password || form.password.length < 6) return '密码至少6位'
  return '密码长度符合要求'
})

const passwordHintClass = computed(() => {
  if (!form.password) return ''
  return form.password.length >= 6 ? 'is-success' : 'is-error'
})

const confirmPasswordHintText = computed(() => {
  if (!form.confirmPassword) return ''
  if (!form.password) return '请先输入新密码'
  return form.password === form.confirmPassword ? '两次密码一致' : '两次密码不一致'
})

const confirmPasswordHintClass = computed(() => {
  if (!form.confirmPassword) return ''
  return form.password && form.password === form.confirmPassword ? 'is-success' : 'is-error'
})

const formatDurationText = seconds => {
  const value = Number(seconds || 0)
  if (value <= 0) return ''
  if (value % 3600 === 0) return `${value / 3600}小时`
  if (value % 60 === 0) return `${value / 60}分钟`
  return `${value}秒`
}

const validateEmail = () => {
  if (!form.email || !emailPattern.test(form.email)) {
    error.value = '请输入有效邮箱'
    return false
  }
  return true
}

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

const openCaptchaModal = async () => {
  error.value = ''
  success.value = ''
  if (!validateEmail()) return

  captchaModalVisible.value = true
  await fetchCaptcha()
  await nextTick()
  captchaInputRef.value?.focus()
}

const closeCaptchaModal = () => {
  if (sendingEmailCode.value) return
  captchaModalVisible.value = false
  form.captchaCode = ''
}

const startCooldown = seconds => {
  cooldownSeconds.value = Number(seconds || 0)
  if (cooldownTimer) {
    clearInterval(cooldownTimer)
  }
  if (cooldownSeconds.value <= 0) return
  cooldownTimer = setInterval(() => {
    cooldownSeconds.value -= 1
    if (cooldownSeconds.value <= 0) {
      clearInterval(cooldownTimer)
      cooldownTimer = null
    }
  }, 1000)
}

const handleSendEmailCode = async () => {
  error.value = ''
  success.value = ''

  if (!validateEmail()) return
  if (!form.captchaCode || !captchaId.value) {
    error.value = '请先输入图片验证码'
    return
  }

  sendingEmailCode.value = true
  try {
    const res = await sendPasswordResetEmailCode(form.email, captchaId.value, form.captchaCode)
    if (res.data.code === 0) {
      emailCodeExpiresSeconds.value = Number(res.data.data?.expires_in_seconds || 0)
      success.value = emailCodeExpiresText.value ? `邮箱验证码已发送，${emailCodeExpiresText.value}后过期` : '邮箱验证码已发送'
      startCooldown(res.data.data?.cooldown_seconds)
      captchaModalVisible.value = false
      form.captchaCode = ''
    } else {
      error.value = res.data.msg || '邮箱验证码发送失败'
      await fetchCaptcha()
    }
  } catch (e) {
    error.value = e.response?.data?.msg || '邮箱验证码发送失败'
    await fetchCaptcha()
  } finally {
    sendingEmailCode.value = false
  }
}

const handleSubmit = async () => {
  error.value = ''
  success.value = ''

  if (!validateEmail()) return
  if (!form.password || form.password.length < 6) {
    error.value = '密码至少6位'
    return
  }
  if (form.password !== form.confirmPassword) {
    error.value = '两次密码不一致'
    return
  }
  if (!form.emailCode) {
    error.value = '请输入邮箱验证码'
    return
  }

  loading.value = true
  try {
    const res = await resetPassword(form.email, form.emailCode, form.password)
    if (res.data.code === 0) {
      success.value = '密码已重置，即将跳转登录...'
      setTimeout(() => router.push('/login'), 1500)
    } else {
      error.value = res.data.msg || '密码重置失败'
    }
  } catch (e) {
    error.value = e.response?.data?.msg || '密码重置失败'
  } finally {
    loading.value = false
  }
}

onBeforeUnmount(() => {
  if (cooldownTimer) {
    clearInterval(cooldownTimer)
  }
})
</script>

<style scoped>
.captcha-row,
.email-code-row {
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

.email-code-button {
  width: 132px;
  min-height: 44px;
  padding: 0 12px;
}

.form-helper {
  margin-top: 8px;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.5;
}

.form-helper.is-success {
  color: var(--success);
}

.form-helper.is-error {
  color: var(--danger);
}

.modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(23, 23, 23, 0.42);
  backdrop-filter: blur(10px);
}

.captcha-modal {
  width: min(100%, 460px);
  padding: 24px;
  border: 1px solid var(--border);
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.96);
  box-shadow: var(--shadow-md);
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
}

.modal-header h2 {
  color: var(--text-primary);
  font-size: 24px;
  line-height: 1.2;
}

.modal-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: #fff;
  color: var(--text-secondary);
  font-size: 28px;
  line-height: 1;
  cursor: pointer;
}

.modal-close:disabled {
  cursor: wait;
  opacity: 0.6;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 20px;
}

@media (max-width: 420px) {
  .captcha-row,
  .email-code-row {
    grid-template-columns: minmax(0, 1fr);
  }

  .captcha-button,
  .captcha-image,
  .email-code-button {
    width: 100%;
  }

  .modal-actions {
    flex-direction: column-reverse;
  }

  .modal-actions .btn {
    width: 100%;
  }
}
</style>
