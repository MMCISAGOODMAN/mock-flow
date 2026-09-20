# MockFlow

单二进制、开箱即用的 Mock API 工具。同一个接口可以按资源创建后的经过时间返回不同响应，适合前端轮询订单状态这类场景。

## 安装

需要 Go 1.22+ 与 Node.js（仅构建前端；运行二进制不需要 Node）。

```bash
make build
./mockflow start
```

启动后输出类似：

```
Config UI:        http://localhost:8080/_ui
LAN UI:           http://192.168.1.8:8080/_ui
Project Default     mock http://localhost:8080 (2 endpoints)
Loaded 2 mock endpoints from ./mockflow.db
```

配置界面按**项目**分组接口。每个项目有独立 Mock 端口（可与 UI 端口相同）。例如商城用 `8081`，订单用 `8082`，其他工程分别把 base URL 指过去。

浏览器会尝试自动打开配置界面。配置保存在 SQLite（默认 `./mockflow.db`），重启进程后接口仍然存在。

跨平台静态二进制：

```bash
make build-all
```

产物在 `dist/`：macOS arm64、Linux amd64、Windows amd64。全程 `CGO_ENABLED=0`，无需 JDK、Python 或 Node 运行时。

## 命令

```
mockflow start [--port 8080] [--db ./mockflow.db]
mockflow restart [--port 8080] [--db ./mockflow.db]
mockflow stop [--port 8080]
mockflow export <file.yaml>
mockflow import <file.yaml>
mockflow version
```

改完配置或重新编译后，用 `./mockflow restart` 会先停掉占用该端口的旧进程再启动。

`import` / `export` 默认读写当前目录的 `./mockflow.db`。

## 给其他项目调用

每个项目的 Mock 基地址是 `http://<host>:<项目端口>`。接口列表顶部可复制当前项目地址。`start --port` 只决定配置 UI；Mock 端口在项目管理里设置。

```bash
curl http://localhost:8080/orders/1
curl http://localhost:8081/orders/1   # 另一个项目
```

Mock 响应带 `Access-Control-Allow-Origin: *`，并处理 `OPTIONS` 预检。`/_ui` 和 `/_api` 只挂在控制端口上。

## 时间状态机

请求命中 `method + path` 后：

1. 从路径参数提取 `resource_key`（如 `/orders/{id}` 访问 `/orders/1` 时为 `1`）
2. 若该资源尚无运行时记录则创建，`created_at = now`
3. `elapsed = now - created_at`（秒）
4. 在 `response_stages` 中取 `after_seconds <= elapsed` 的最大阶段
5. 若 elapsed 小于所有阶段，返回第一个阶段；超过全部阶段则返回最后一个

`POST` 会重置该资源的计时。`DELETE` 在返回 mock 响应后删除该 `resource_key` 的运行时状态。

路径支持 `{id}` 参数和 `/api/*` 通配符；更具体的模式优先。body / headers 中的 `{id}` 会替换为实际路径参数。`/_ui` 与 `/_api` 为保留前缀。

## YAML 格式

`version: 1` 仍可用，会导入到 Default 项目（端口 8080）。推荐 `version: 2`：

```yaml
version: 2
projects:
  - name: shop
    port: 8081
    description: 商城
    endpoints:
      - method: GET
        path: /orders/{id}
        description: 订单状态查询
        stages:
          - after_seconds: 0
            status_code: 200
            headers:
              Content-Type: application/json
            body:
              id: "{id}"
              status: processing
          - after_seconds: 5
            status_code: 200
            body:
              id: "{id}"
              status: shipped
          - after_seconds: 30
            status_code: 200
            body:
              id: "{id}"
              status: delivered
      - method: POST
        path: /orders
        description: 创建订单
        stages:
          - after_seconds: 0
            status_code: 201
            body:
              id: "1"
              status: created
```

导入失败会返回带行号的错误。完整示例见 [examples/orders.yaml](examples/orders.yaml)。

```bash
./mockflow import examples/orders.yaml
./mockflow start
```

在配置界面创建 `GET /orders/{id}` 的 0s / 5s / 30s 三阶段后，打开「调用测试」请求 `/orders/1`：第一次为 `processing`，约 5 秒后再发为 `shipped`。
