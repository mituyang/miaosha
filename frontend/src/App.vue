<template>
  <div class="app">
    <!-- 导航栏 (登录后显示) -->
    <nav v-if="showNavbar" class="navbar">
      <router-link to="/seckill" class="nav-brand">秒杀系统</router-link>
      <div class="nav-links">
        <router-link to="/seckill" class="nav-link">秒杀活动</router-link>
        <router-link to="/orders" class="nav-link">我的订单</router-link>
      </div>
      <div class="nav-user">
        <span v-if="isLoggedIn" class="username">{{ username }}</span>
        <button v-if="isLoggedIn" class="btn-logout" @click="handleLogout">退出</button>
      </div>
    </nav>

    <!-- 主内容 -->
    <main class="main-content">
      <router-view />
    </main>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const router = useRouter()
const route = useRoute()

const token = ref(localStorage.getItem('token') || '')
const username = ref(localStorage.getItem('username') || '')
const isLoggedIn = computed(() => !!token.value)
const showNavbar = computed(() => isLoggedIn.value)

const syncAuthState = () => {
  token.value = localStorage.getItem('token') || ''
  username.value = localStorage.getItem('username') || ''
}

watch(() => route.fullPath, syncAuthState, { immediate: true })

const handleLogout = () => {
  localStorage.removeItem('token')
  localStorage.removeItem('username')
  syncAuthState()
  router.push('/login')
}
</script>
