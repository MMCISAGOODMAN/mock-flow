<template>
  <div class="page">
    <div class="page-head">
      <h2>项目管理</h2>
      <el-button type="primary" @click="openEdit()">新建项目</el-button>
    </div>
    <el-table :data="projects" stripe empty-text="还没有项目">
      <el-table-column prop="name" label="名称" min-width="140" />
      <el-table-column label="Mock 端口" width="120">
        <template #default="{ row }">
          <span class="mono">:{{ row.port }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="description" label="描述" min-width="180" />
      <el-table-column prop="endpoint_count" label="接口数" width="90" />
      <el-table-column label="操作" width="180">
        <template #default="{ row }">
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" @click="onDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="visible" :title="form.id ? '编辑项目' : '新建项目'" width="480px">
      <el-form label-width="90px">
        <el-form-item label="名称">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="端口">
          <el-input-number v-model="form.port" :min="1024" :max="65535" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="visible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { inject, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { createProject, deleteProject, updateProject } from '../api'

const projects = inject('projects')
const reloadProjects = inject('reloadProjects')
const visible = ref(false)
const saving = ref(false)
const form = reactive({ id: null, name: '', port: 8081, description: '' })

function openEdit(row) {
  if (row) {
    form.id = row.id
    form.name = row.name
    form.port = row.port
    form.description = row.description
  } else {
    form.id = null
    form.name = ''
    form.port = nextPort()
    form.description = ''
  }
  visible.value = true
}

function nextPort() {
  const used = new Set((projects.value || []).map((p) => p.port))
  let p = 8081
  while (used.has(p)) p += 1
  return p
}

async function save() {
  if (!form.name) {
    ElMessage.error('名称必填')
    return
  }
  saving.value = true
  try {
    const payload = { name: form.name, port: form.port, description: form.description }
    if (form.id) {
      await updateProject(form.id, payload)
    } else {
      await createProject(payload)
    }
    ElMessage.success('已保存')
    visible.value = false
    await reloadProjects()
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    saving.value = false
  }
}

async function onDelete(row) {
  try {
    await ElMessageBox.confirm(`删除项目 ${row.name} 及其全部接口？`, '确认删除', { type: 'warning' })
    await deleteProject(row.id)
    ElMessage.success('已删除')
    await reloadProjects()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(e.message || String(e))
  }
}
</script>
