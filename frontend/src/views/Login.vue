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
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { login } from '../api'

const router = useRouter()
const loading = ref(false)
const error = ref('')
const form = reactive({ username: '', password: '' })

const handleSubmit = async () => {
  error.value = ''
  
  if (!form.username || !form.password) {
    error.value = '请填写用户名和密码'
    return
  }
  
  loading.value = true
  try {
    const res = await login(form.username, form.password)
    if (res.data.code === 0) {
      localStorage.setItem('token', res.data.data.token)
      localStorage.setItem('username', form.username)
      router.push('/seckill')
    } else {
      error.value = res.data.msg
    }
  } catch (e) {
    error.value = e.response?.data?.msg || '登录失败'
  } finally {
    loading.value = false
  }
}
</script>
