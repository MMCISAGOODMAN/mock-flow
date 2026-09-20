<template>
  <div class="page">
    <div class="page-head">
      <h2>接口列表</h2>
      <div class="actions">
        <el-input
          v-model="query"
          class="search"
          clearable
          placeholder="搜索 method / path / 描述"
        />
        <el-button @click="$router.push('/endpoints/new')" type="primary">新建接口</el-button>
        <el-button @click="onImport">导入 YAML</el-button>
        <el-button @click="onExport">导出 YAML</el-button>
        <input ref="fileInput" type="file" accept=".yaml,.yml,text/yaml" hidden @change="onFile" />
      </div>
    </div>
    <el-table :data="filteredRows" v-loading="loading" stripe empty-text="没有匹配的 Mock 接口">
      <el-table-column label="Method" width="110">
        <template #default="{ row }">
          <el-tag :type="methodType(row.method)" size="small">{{ row.method }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Path" min-width="220">
        <template #default="{ row }">
          <span class="mono">{{ row.path }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="description" label="描述" min-width="180" />
      <el-table-column prop="stage_count" label="阶段数" width="90" />
      <el-table-column label="创建时间" width="190">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="280" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="$router.push(`/endpoints/${row.id}`)">编辑</el-button>
          <el-button link type="primary" @click="onCopy(row)">复制</el-button>
          <el-button link type="primary" @click="onTest(row)">测试</el-button>
          <el-button link type="danger" @click="onDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup>
import { computed, inject, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  deleteEndpoint,
  downloadText,
  duplicateEndpoint,
  exportYaml,
  importYaml,
  listEndpoints
} from '../api'

const router = useRouter()
const currentProject = inject('currentProject')
const reloadProjects = inject('reloadProjects')
const rows = ref([])
const query = ref('')
const loading = ref(false)
const fileInput = ref(null)
const filteredRows = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return rows.value
  return rows.value.filter((row) => {
    const hay = [row.method, row.path, row.description, String(row.stage_count ?? '')]
      .join(' ')
      .toLowerCase()
    return hay.includes(q)
  })
})

function methodType(m) {
  const map = { GET: 'success', POST: 'primary', PUT: 'warning', PATCH: 'warning', DELETE: 'danger' }
  return map[m] || 'info'
}

function formatTime(v) {
  if (!v) return ''
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return v
  return d.toLocaleString()
}

async function load() {
  loading.value = true
  try {
    rows.value = await listEndpoints(currentProject.value?.id)
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    loading.value = false
  }
}

async function onDelete(row) {
  try {
    await ElMessageBox.confirm(`删除 ${row.method} ${row.path} ?`, '确认删除', { type: 'warning' })
    await deleteEndpoint(row.id)
    ElMessage.success('已删除')
    await load()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(e.message || String(e))
  }
}

async function onCopy(row) {
  try {
    await duplicateEndpoint(row.id)
    ElMessage.success('已复制')
    await load()
  } catch (e) {
    ElMessage.error(e.message)
  }
}

function onTest(row) {
  router.push({ path: '/test', query: { endpoint_id: row.id } })
}

function onImport() {
  fileInput.value?.click()
}

async function onFile(ev) {
  const file = ev.target.files?.[0]
  ev.target.value = ''
  if (!file) return
  try {
    const text = await file.text()
    const res = await importYaml(text)
    ElMessage.success(`已导入 ${res.imported} 个接口`)
    await reloadProjects?.()
    await load()
  } catch (e) {
    ElMessage.error(e.message)
  }
}

async function onExport() {
  try {
    const text = await exportYaml()
    downloadText('mockflow.yaml', text)
  } catch (e) {
    ElMessage.error(e.message)
  }
}

onMounted(load)
watch(() => currentProject.value?.id, load)

</script>

<style scoped>
.actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.search {
  width: 240px;
}
</style>
