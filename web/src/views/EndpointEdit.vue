<template>
  <div class="page" v-loading="loading">
    <div class="page-head">
      <h2>{{ isNew ? '新建接口' : '编辑接口' }}</h2>
      <div>
        <el-button @click="$router.push('/')">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </div>
    </div>

    <el-card shadow="never">
      <el-form label-width="110px">
        <el-form-item label="Method">
          <el-select v-model="form.method" style="width: 180px">
            <el-option v-for="m in methods" :key="m" :label="m" :value="m" />
          </el-select>
        </el-form-item>
        <el-form-item label="Path">
          <el-input v-model="form.path" placeholder="/orders/{id}" class="mono" />
        </el-form-item>
        <el-form-item label="Description">
          <el-input v-model="form.description" placeholder="接口说明" />
        </el-form-item>
      </el-form>
    </el-card>

    <div class="stage-head">
      <h3>响应阶段</h3>
      <el-button type="primary" plain @click="addStage">添加阶段</el-button>
    </div>
    <p class="hint">按 after_seconds 匹配阶段；拖拽仅调整编辑顺序。elapsed 小于所有阶段时返回第一个阶段。</p>

    <draggable v-model="form.stages" item-key="_key" handle=".drag-handle" class="stages">
      <template #item="{ element, index }">
        <el-card class="stage-card" shadow="never">
          <div class="stage-top">
            <span class="drag-handle">拖拽</span>
            <strong>阶段 {{ index + 1 }}</strong>
            <el-button link type="danger" @click="removeStage(index)">删除阶段</el-button>
          </div>
          <el-form label-width="130px">
            <el-form-item label="after_seconds">
              <el-input-number v-model="element.after_seconds" :min="0" :step="1" />
            </el-form-item>
            <el-form-item label="status_code">
              <el-input-number v-model="element.status_code" :min="100" :max="599" :step="1" />
            </el-form-item>
            <el-form-item label="headers (JSON)">
              <el-input v-model="element.headers" type="textarea" :rows="4" class="mono" />
              <el-button class="fmt" link type="primary" @click="formatJSON(element, 'headers')">格式化</el-button>
            </el-form-item>
            <el-form-item label="body (JSON)">
              <el-input v-model="element.body" type="textarea" :rows="8" class="mono" />
              <el-button class="fmt" link type="primary" @click="formatJSON(element, 'body')">格式化</el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </template>
    </draggable>
  </div>
</template>

<script setup>
import { computed, inject, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import draggable from 'vuedraggable'
import { createEndpoint, getEndpoint, updateEndpoint } from '../api'

const methods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE']
const currentProject = inject('currentProject')
const route = useRoute()
const router = useRouter()
const loading = ref(false)
const saving = ref(false)
const form = reactive({
  method: 'GET',
  path: '',
  description: '',
  stages: []
})

const isNew = computed(() => !route.params.id)

let keySeq = 1
function newStage(partial = {}) {
  return {
    _key: keySeq++,
    after_seconds: partial.after_seconds ?? 0,
    status_code: partial.status_code ?? 200,
    headers: pretty(partial.headers || '{"Content-Type":"application/json"}'),
    body: pretty(partial.body || '{}')
  }
}

function pretty(raw) {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw || ''
  }
}

function addStage() {
  const last = form.stages[form.stages.length - 1]
  const after = last ? Number(last.after_seconds || 0) + 5 : 0
  form.stages.push(newStage({ after_seconds: after }))
}

function removeStage(i) {
  form.stages.splice(i, 1)
}

function formatJSON(el, field) {
  try {
    el[field] = JSON.stringify(JSON.parse(el[field] || (field === 'headers' ? '{}' : '{}')), null, 2)
  } catch (e) {
    ElMessage.error(`${field} 不是合法 JSON`)
  }
}

function toPayload() {
  return {
    project_id: currentProject.value?.id,
    method: form.method,
    path: form.path,
    description: form.description,
    stages: form.stages.map((st) => ({
      after_seconds: Number(st.after_seconds || 0),
      status_code: Number(st.status_code || 200),
      headers: st.headers || '{}',
      body: st.body || ''
    }))
  }
}

async function save() {
  if (!form.path) {
    ElMessage.error('path 必填')
    return
  }
  for (const st of form.stages) {
    try {
      JSON.parse(st.headers || '{}')
    } catch {
      ElMessage.error('headers 必须是 JSON 对象')
      return
    }
  }
  saving.value = true
  try {
    const payload = toPayload()
    if (isNew.value) {
      await createEndpoint(payload)
    } else {
      await updateEndpoint(route.params.id, payload)
    }
    ElMessage.success('已保存')
    router.push('/')
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  if (isNew.value) {
    form.stages = [newStage({ body: '{\n  "id": "{id}",\n  "status": "processing"\n}' })]
    return
  }
  loading.value = true
  try {
    const e = await getEndpoint(route.params.id)
    form.method = e.method
    form.path = e.path
    form.description = e.description
    form.stages = (e.stages || []).map((st) => newStage(st))
  } catch (err) {
    ElMessage.error(err.message)
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.stage-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 20px 0 8px;
}
.stage-head h3 {
  margin: 0;
}
.hint {
  color: #909399;
  margin: 0 0 12px;
  font-size: 13px;
}
.stages {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.stage-card {
  border: 1px solid #ebeef5;
}
.stage-top {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}
.drag-handle {
  cursor: grab;
  color: #409eff;
  font-size: 13px;
}
.fmt {
  margin-left: 8px;
}
</style>
