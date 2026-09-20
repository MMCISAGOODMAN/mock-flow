<template>
  <div class="page">
    <div class="page-head">
      <h2>调用测试</h2>
    </div>
    <el-row :gutter="16">
      <el-col :span="12">
        <el-card shadow="never">
          <el-form label-width="90px">
            <el-form-item label="接口">
              <el-select v-model="selectedId" filterable placeholder="选择已配置接口" style="width: 100%" @change="applyEndpoint">
                <el-option
                  v-for="e in endpoints"
                  :key="e.id"
                  :label="`${e.method} ${e.path}`"
                  :value="e.id"
                />
              </el-select>
            </el-form-item>
            <el-form-item label="Method">
              <el-select v-model="req.method" style="width: 160px">
                <el-option v-for="m in methods" :key="m" :label="m" :value="m" />
              </el-select>
            </el-form-item>
            <el-form-item label="Path">
              <el-input v-model="req.path" placeholder="/orders/1" class="mono" />
            </el-form-item>
            <el-form-item label="Headers">
              <el-input v-model="req.headers" type="textarea" :rows="5" class="mono" />
            </el-form-item>
            <el-form-item label="Body">
              <el-input v-model="req.body" type="textarea" :rows="8" class="mono" />
            </el-form-item>
            <el-form-item label="curl">
              <pre class="mono block curl">{{ curlCommand }}</pre>
              <el-button class="copy-curl" link type="primary" @click="copyCurl">复制 curl</el-button>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="sending" @click="send">发送</el-button>
              <el-button @click="reset">重置状态</el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card shadow="never">
          <template #header>响应</template>
          <el-descriptions :column="1" border v-if="result">
            <el-descriptions-item label="状态码">{{ result.status_code || '-' }}</el-descriptions-item>
            <el-descriptions-item label="当前阶段">
              {{ result.after_seconds == null ? '-' : `${result.after_seconds}s (stage ${result.stage_id})` }}
            </el-descriptions-item>
            <el-descriptions-item label="elapsed">{{ result.elapsed_seconds }} 秒</el-descriptions-item>
            <el-descriptions-item label="resource_key">{{ result.resource_key }}</el-descriptions-item>
            <el-descriptions-item label="错误" v-if="result.error">{{ result.error }}</el-descriptions-item>
          </el-descriptions>
          <h4>Headers</h4>
          <pre class="mono block">{{ pretty(result?.headers) }}</pre>
          <h4>Body</h4>
          <pre class="mono block">{{ formatBody(result?.body) }}</pre>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { computed, inject, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getEndpoint, invoke, listEndpoints, mockBaseURL, resetRuntime } from '../api'

const methods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE']
const currentProject = inject('currentProject')
const route = useRoute()
const endpoints = ref([])
const selectedId = ref(null)
const sending = ref(false)
const result = ref(null)
const req = reactive({
  method: 'GET',
  path: '/orders/1',
  headers: '{\n  "Accept": "application/json"\n}',
  body: ''
})

function shellQuote(s) {
  return `'${String(s).replace(/'/g, `'\\''`)}'`
}

const curlCommand = computed(() => {
  let path = req.path || '/'
  if (!path.startsWith('/')) path = `/${path}`
  const url = `${mockBaseURL(currentProject.value)}${path}`
  const lines = ['curl']
  const method = (req.method || 'GET').toUpperCase()
  if (method !== 'GET') {
    lines.push(`  -X ${method}`)
  }
  let headers = {}
  try {
    headers = JSON.parse(req.headers || '{}')
  } catch {
    headers = {}
  }
  for (const [k, v] of Object.entries(headers)) {
    if (k === '') continue
    lines.push(`  -H ${shellQuote(`${k}: ${v}`)}`)
  }
  if (req.body && method !== 'GET' && method !== 'HEAD') {
    lines.push(`  --data-raw ${shellQuote(req.body)}`)
  }
  lines.push(`  ${shellQuote(url)}`)
  if (lines.length === 2) {
    return `curl ${shellQuote(url)}`
  }
  lines[0] = 'curl \\'
  return lines
    .map((line, i) => (i === 0 || i === lines.length - 1 ? line : `${line} \\`))
    .join('\n')
})

async function copyCurl() {
  try {
    await navigator.clipboard.writeText(curlCommand.value)
    ElMessage.success('已复制 curl')
  } catch (e) {
    ElMessage.error(e.message || '复制失败')
  }
}

function pretty(v) {
  try {
    return JSON.stringify(v ?? {}, null, 2)
  } catch {
    return String(v)
  }
}

function formatBody(body) {
  if (body == null || body === '') return ''
  try {
    return JSON.stringify(JSON.parse(body), null, 2)
  } catch {
    return body
  }
}

function samplePath(path) {
  return (path || '').replace(/\{[^/]+\}/g, '1').replace(/\/\*$/, '/demo')
}

async function applyEndpoint(id) {
  const e = endpoints.value.find((x) => x.id === id)
  if (!e) return
  req.method = e.method
  req.path = samplePath(e.path)
  try {
    const full = await getEndpoint(id)
    selectedId.value = id
    req.method = full.method
    req.path = samplePath(full.path)
  } catch (err) {
    ElMessage.error(err.message)
  }
}

async function send() {
  let headers = {}
  try {
    headers = JSON.parse(req.headers || '{}')
  } catch {
    ElMessage.error('headers 必须是 JSON 对象')
    return
  }
  sending.value = true
  try {
    result.value = await invoke({
      method: req.method,
      path: req.path,
      headers,
      body: req.body,
      project_id: currentProject.value?.id
    })
    if (result.value.endpoint_id) {
      selectedId.value = result.value.endpoint_id
    }
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    sending.value = false
  }
}

async function reset() {
  const id = selectedId.value || result.value?.endpoint_id
  const key = result.value?.resource_key
  if (!id) {
    ElMessage.warning('请先选择接口或发送一次请求')
    return
  }
  try {
    await resetRuntime(id, key || '_')
    ElMessage.success('已重置 runtime 状态')
    result.value = null
  } catch (e) {
    ElMessage.error(e.message)
  }
}

async function loadEndpoints() {
  try {
    endpoints.value = await listEndpoints(currentProject.value?.id)
    const qid = Number(route.query.endpoint_id)
    if (qid && endpoints.value.some((e) => e.id === qid)) {
      selectedId.value = qid
      await applyEndpoint(qid)
    }
  } catch (e) {
    ElMessage.error(e.message)
  }
}

onMounted(loadEndpoints)
watch(() => currentProject.value?.id, loadEndpoints)
</script>

<style scoped>
.block {
  background: #f5f7fa;
  padding: 12px;
  border-radius: 6px;
  overflow: auto;
  min-height: 80px;
}
.curl {
  margin: 0;
  min-height: 0;
  white-space: pre-wrap;
  word-break: break-all;
}
.copy-curl {
  margin-top: 4px;
}
</style>
