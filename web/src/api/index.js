const jsonHeaders = { 'Content-Type': 'application/json' }

async function parse(res) {
  if (res.status === 204) return null
  const text = await res.text()
  let data = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = { error: text }
  }
  if (!res.ok) {
    throw new Error(data?.error || res.statusText)
  }
  return data
}

export function listProjects() {
  return fetch('/_api/projects').then(parse)
}

export function getProject(id) {
  return fetch(`/_api/projects/${id}`).then(parse)
}

export function createProject(payload) {
  return fetch('/_api/projects', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(payload)
  }).then(parse)
}

export function updateProject(id, payload) {
  return fetch(`/_api/projects/${id}`, {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify(payload)
  }).then(parse)
}

export function deleteProject(id) {
  return fetch(`/_api/projects/${id}`, { method: 'DELETE' }).then(parse)
}

export function getMeta() {
  return fetch('/_api/meta').then(parse)
}

export function listEndpoints(projectId) {
  const q = projectId ? `?project_id=${projectId}` : ''
  return fetch(`/_api/endpoints${q}`).then(parse)
}

export function getEndpoint(id) {
  return fetch(`/_api/endpoints/${id}`).then(parse)
}

export function createEndpoint(payload) {
  return fetch('/_api/endpoints', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(payload)
  }).then(parse)
}

export function updateEndpoint(id, payload) {
  return fetch(`/_api/endpoints/${id}`, {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify(payload)
  }).then(parse)
}

export function deleteEndpoint(id) {
  return fetch(`/_api/endpoints/${id}`, { method: 'DELETE' }).then(parse)
}

export function duplicateEndpoint(id) {
  return fetch(`/_api/endpoints/${id}/duplicate`, { method: 'POST' }).then(parse)
}

export function invoke(payload) {
  return fetch('/_api/invoke', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(payload)
  }).then(parse)
}

export function resetRuntime(endpointId, resourceKey) {
  const q = new URLSearchParams({
    endpoint_id: String(endpointId),
    resource_key: resourceKey || '_'
  })
  return fetch(`/_api/runtime?${q}`, { method: 'DELETE' }).then(parse)
}

export function exportYaml() {
  return fetch('/_api/export').then(async (res) => {
    if (!res.ok) {
      const t = await res.text()
      throw new Error(t || 'export failed')
    }
    return res.text()
  })
}

export function importYaml(text) {
  return fetch('/_api/import', {
    method: 'POST',
    headers: { 'Content-Type': 'application/yaml' },
    body: text
  }).then(parse)
}

export function downloadText(filename, text) {
  const blob = new Blob([text], { type: 'application/yaml' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

export function mockBaseURL(project) {
  const host = window.location.hostname || 'localhost'
  const port = project?.port || 8080
  const proto = window.location.protocol === 'https:' ? 'https:' : 'http:'
  return `${proto}//${host}:${port}`
}
