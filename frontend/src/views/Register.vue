<template>
  <div class="auth-page">
    <div class="auth-card">
      <div class="auth-copy">
        <span class="auth-kicker">Miaosha</span>
        <h1 class="auth-title">创建账号</h1>
        <p class="auth-subtitle">注册后即可进入秒杀前台，参与抢购并追踪自己的订单进度。</p>
      </div>

      <div v-if="error" class="alert alert-error">{{ error }}</div>
      <div v-if="success" class="alert alert-success">{{ success }}</div>

      <form @submit.prevent="handleSubmit" class="auth-form">
        <div class="form-group">
          <label class="form-label" for="register-username">用户名</label>
          <input
            id="register-username"
            v-model="form.username"
            type="text"
            class="form-input"
            placeholder="至少3个字符"
            autocomplete="username"
          />
        </div>

        <div class="form-group">
          <label class="form-label" for="register-password">密码</label>
          <input
            id="register-password"
            v-model="form.password"
            type="password"
            class="form-input"
            placeholder="至少6个字符"
            autocomplete="new-password"
          />
        </div>

        <div class="form-group">
          <label class="form-label" for="register-confirm-password">确认密码</label>
          <input
            id="register-confirm-password"
            v-model="form.confirmPassword"
            type="password"
            class="form-input"
            placeholder="再次输入密码"
            autocomplete="new-password"
          />
        </div>

        <button type="submit" class="btn btn-primary btn-block" :disabled="loading">
          {{ loading ? '注册中...' : '注册' }}
        </button>
      </form>

      <div class="auth-footer">
        已有账号？<router-link to="/login" class="link">立即登录</router-link>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { register } from '../api'

const router = useRouter()
const loading = ref(false)
const error = ref('')
const success = ref('')
const form = reactive({ username: '', password: '', confirmPassword: '' })

const handleSubmit = async () => {
  error.value = ''
  success.value = ''
  
  if (!form.username || form.username.length < 3) {
    error.value = '用户名至少3个字符'
    return
  }
  if (!form.password || form.password.length < 6) {
    error.value = '密码至少6个字符'
    return
  }
  if (form.password !== form.confirmPassword) {
    error.value = '两次密码不一致'
    return
  }
  
  loading.value = true
  try {
    const res = await register(form.username, form.password)
    if (res.data.code === 0) {
      success.value = '注册成功，即将跳转登录...'
      setTimeout(() => router.push('/login'), 1500)
    } else {
      error.value = res.data.msg
    }
  } catch (e) {
    error.value = e.response?.data?.msg || '注册失败'
  } finally {
    loading.value = false
  }
}
</script>
