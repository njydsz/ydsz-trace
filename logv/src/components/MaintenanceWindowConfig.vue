<template>
  <div class="maintenance-config">
    <div class="flex items-center justify-between mb-3">
      <p class="text-sm text-muted">
        计划内维护期间（发布、巡检）整体静默告警，支持跨天窗口与周周期。
      </p>
      <div class="flex items-center gap-2">
        <el-button size="small" text type="primary" @click="addWindow">新增窗口</el-button>
        <el-button size="small" type="primary" :loading="saving" @click="onSave">保存并同步</el-button>
      </div>
    </div>

    <el-empty v-if="!windows.length" description="暂无静默窗口配置" :image-size="60" />

    <div v-else class="space-y-3">
      <div
        v-for="(w, idx) in windows"
        :key="w._key"
        class="flex flex-wrap items-center gap-2 p-3 rounded border border-gray-200 dark:border-gray-700"
      >
        <el-input v-model="w.name" placeholder="窗口名称" size="small" style="width: 160px" />

        <el-select v-model="w.recurring" size="small" style="width: 110px">
          <el-option label="每天" value="daily" />
          <el-option label="每周" value="weekly" />
          <el-option label="一次性" value="once" />
        </el-select>

        <el-time-select
          v-model="w.startTime"
          start="00:00"
          end="23:59"
          step="00:30"
          placeholder="开始"
          size="small"
          style="width: 100px"
        />
        <span class="text-xs text-muted">至</span>
        <el-time-select
          v-model="w.endTime"
          start="00:00"
          end="23:59"
          step="00:30"
          placeholder="结束"
          size="small"
          style="width: 100px"
        />

        <el-select
          v-if="w.recurring === 'weekly'"
          v-model="w.weekdays"
          multiple
          collapse-tags
          placeholder="选择星期"
          size="small"
          style="width: 200px"
        >
          <el-option v-for="(d, i) in weekdayOptions" :key="i" :label="d" :value="i" />
        </el-select>

        <el-switch v-model="w.enabled" size="small" />

        <el-button size="small" text type="danger" @click="removeWindow(idx)">删除</el-button>
      </div>
    </div>
  </div>
</template>

<script setup>
// 静默窗口配置（V2.2-02）
//
// 保存后调用 syncMaintenanceWindows 让运行中的调度器立即生效，
// 避免「保存了但不生效」的困惑。
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { listMaintenanceWindows, saveMaintenanceWindows, syncMaintenanceWindows } from '@/api/alert'

const windows = ref([])
const saving = ref(false)

const weekdayOptions = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']

// 稳定 key 用于 v-for（避免删除时复用错误 DOM）
let keySeq = 0

function nextKey() {
  keySeq += 1
  return `w-${keySeq}`
}

function addWindow() {
  windows.value.push({
    _key: nextKey(),
    name: '',
    startTime: '02:00',
    endTime: '04:00',
    recurring: 'daily',
    weekdays: [],
    enabled: true,
  })
}

function removeWindow(idx) {
  windows.value.splice(idx, 1)
}

async function load() {
  try {
    const { data } = await listMaintenanceWindows()
    const list = data.data?.windows || []
    windows.value = list.map((w) => ({ ...w, _key: nextKey() }))
  } catch {
    windows.value = []
  }
}

function validate() {
  for (const w of windows.value) {
    if (!w.name.trim()) {
      ElMessage.warning('请填写窗口名称')
      return false
    }
    if (!/^\d{2}:\d{2}$/.test(w.startTime) || !/^\d{2}:\d{2}$/.test(w.endTime)) {
      ElMessage.warning('时间格式应为 HH:MM')
      return false
    }
    if (w.recurring === 'weekly' && (!w.weekdays || w.weekdays.length === 0)) {
      ElMessage.warning('每周窗口需选择星期')
      return false
    }
  }
  return true
}

async function onSave() {
  if (!validate()) return
  saving.value = true
  try {
    const payload = windows.value.map(({ _key, ...rest }) => rest)
    await saveMaintenanceWindows(payload)
    await syncMaintenanceWindows()
    ElMessage.success('已保存并同步到运行中的调度器')
  } catch {
    ElMessage.error('保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.maintenance-config {
  width: 100%;
}
</style>
