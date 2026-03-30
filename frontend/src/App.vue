<template>
  <div class="app">
    <!-- 导航栏 (登录后显示) -->
    <nav v-if="showNavbar" class="navbar">
      <router-link to="/seckill" class="nav-brand">秒杀系统</router-link>
      <div class="nav-links">
        <router-link to="/seckill" class="nav-link">秒杀商品</router-link>
        <router-link to="/orders" class="nav-link">我的订单</router-link>
        <a href="/admin/" class="nav-link">后台管理</a>
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
import { computed } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()

const isLoggedIn = computed(() => !!localStorage.getItem('token'))
const username = computed(() => localStorage.getItem('username') || '')
const showNavbar = computed(() => isLoggedIn.value)

const handleLogout = () => {
  localStorage.removeItem('token')
  localStorage.removeItem('username')
  router.push('/login')
}
</script>
