<template>
  <div class="min-h-screen flex items-center justify-center bg-gradient-to-br from-brand-600 to-brand-800">
    <div class="w-full max-w-md p-8 bg-white rounded-2xl shadow-xl">
      <div class="text-center mb-8">
        <h1 class="text-2xl font-bold text-brand-700">Ydsz Trace</h1>
        <p class="text-gray-500 mt-1">分布式日志追踪检索控制台</p>
      </div>
      <el-form :model="form" label-position="top" @submit.prevent="onSubmit">
        <el-form-item label="用户名">
          <el-input v-model="form.username" size="large" placeholder="请输入管理员用户名" :prefix-icon="User" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input
            v-model="form.password"
            type="password"
            size="large"
            show-password
            placeholder="请输入密码"
            :prefix-icon="Lock"
            @keyup.enter="onSubmit"
          />
        </el-form-item>
        <el-button type="primary" size="large" class="w-full !mt-2" :loading="loading" @click="onSubmit">
          登 录
        </el-button>
      </el-form>
    </div>
  </div>
</template>

<script setup>
/**
 * 登录页：管理员账号密码登录。
 *
 * 登录成功后写 session + localStorage，并按 router query 中 redirect 回跳。
 */
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { User, Lock } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { login } from '@/api/auth'
import { useAuth } from '@/store/auth'

const form = ref({ username: '', password: '' })
const loading = ref(false)
const auth = useAuth()
const route = useRoute()
const router = useRouter()

async function onSubmit() {
  if (!form.value.username || !form.value.password) {
    ElMessage.warning('请输入用户名和密码')
    return
  }
  loading.value = true
  try {
    const { data } = await login(form.value.username, form.value.password)
    if (data && data.code === '200') {
      auth.setUser(data.data?.username || form.value.username)
      ElMessage.success(data.msg || '登录成功')
      router.push(route.query.redirect || '/dashboard')
    } else {
      ElMessage.error(data?.msg || '登录失败')
    }
  } catch {
    ElMessage.error('登录请求失败，请确认服务端已启动且可访问')
  } finally {
    loading.value = false
  }
}
</script>
