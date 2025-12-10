<template>
  <div class="app">
    <!-- 导航栏 (登录后显示) -->
    <nav v-if="isLoggedIn" class="navbar">
      <div class="nav-brand">秒杀系统</div>
      <div class="nav-links">
        <router-link to="/seckill" class="nav-link">秒杀商品</router-link>
        <router-link to="/orders" class="nav-link">我的订单</router-link>
      </div>
      <div class="nav-user">
        <span class="username">{{ username }}</span>
        <button class="btn-logout" @click="handleLogout">退出</button>
      </div>
    </nav>

    <!-- 主内容 -->
    <main class="main-content">
      <router-view />
    </main>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()

const isLoggedIn = computed(() => !!localStorage.getItem('token'))
const username = computed(() => localStorage.getItem('username') || '')

const handleLogout = () => {
  localStorage.removeItem('token')
  localStorage.removeItem('username')
  router.push('/login')
}
</script>
