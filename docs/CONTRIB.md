# 开发工作流指南

本文档介绍 Top1000 项目的开发流程和规范。

**最后更新:** 2026-01-30

## 目录

- [开发环境搭建](#开发环境搭建)
- [项目结构](#项目结构)
- [开发命令](#开发命令)
- [代码规范](#代码规范)
- [测试指南](#测试指南)
- [提交规范](#提交规范)
- [问题排查](#问题排查)

## 开发环境搭建

### 环境要求

- Go 1.26+
- Node.js 24.3+
- pnpm 10.12+
- Redis 7.0+

### 后端开发环境

```bash
# 1. 安装 Air 热重载工具（可选）
go install github.com/air-verse/air@latest

# 2. 配置环境变量
cd server
cp .env.example .env
# 编辑 .env 文件，配置 Redis 连接

# 3. 安装依赖
go mod download

# 4. 启动开发服务器
# 方式一：使用 Air 热重载（推荐）
air

# 方式二：直接运行
go run ./cmd/top1000/main.go
```

### 前端开发环境

```bash
# 1. 进入前端目录
cd web

# 2. 安装依赖
pnpm install

# 3. 启动开发服务器
pnpm dev
```

前端开发服务器会在 `http://localhost:5173` 启动，并自动代理后端 API 请求到 `127.0.0.1:7066`。

## 项目结构

```
top1000/
├── server/                 # Go 后端服务
│   ├── cmd/
│   │   └── top1000/       # 应用入口
│   ├── internal/          # 内部模块
│   │   ├── api/          # HTTP API 处理器
│   │   ├── crawler/      # 数据爬取
│   │   ├── server/       # Fiber 服务器
│   │   ├── config/       # 配置管理
│   │   ├── model/        # 数据模型
│   │   └── storage/      # Redis 存储
│   ├── docs/             # API 文档
│   ├── .air.toml         # Air 热重载配置
│   └── go.mod
├── web/                   # TypeScript 前端
│   ├── src/              # 源代码
│   ├── dist/             # 构建产物
│   ├── package.json
│   └── vite.config.ts
├── docs/                 # 项目文档
├── Dockerfile            # 容器构建
├── docker-compose.yaml   # 容器编排
└── .env.example          # 环境变量示例
```

## 开发命令

### 后端命令（在 server 目录）

| 命令 | 说明 |
|------|------|
| `air` | 启动热重载开发服务器 |
| `go run ./cmd/top1000/main.go` | 直接运行应用 |
| `go test ./...` | 运行所有测试 |
| `go test -v ./...` | 运行测试（详细输出） |
| `go test -cover ./...` | 运行测试并显示覆盖率 |
| `go build -o top1000 ./cmd/top1000` | 构建二进制文件 |

### 前端命令（在 web 目录）

| 命令 | 说明 |
|------|------|
| `pnpm install` | 安装依赖 |
| `pnpm dev` | 启动开发服务器 |
| `pnpm build` | 生产构建 |
| `pnpm lint` | 代码检查和修复 |
| `pnpm preview` | 预览构建产物 |

### Docker 命令

| 命令 | 说明 |
|------|------|
| `docker build -t top1000 .` | 构建镜像 |
| `docker-compose up -d` | 启动服务 |
| `docker-compose down` | 停止服务 |
| `docker-compose logs -f` | 查看日志 |

## 代码规范

### Go 代码规范

1. **遵循 Go 官方规范**
   - 使用 `gofmt` 格式化代码
   - 遵循 Effective Go 指南
   - 导出函数必须有注释

2. **错误处理**
   - 始终处理错误，不忽略
   - 使用 `fmt.Errorf` 包装错误
   - 日志记录包含上下文信息

3. **并发安全**
   - 共享状态使用 `sync.Mutex` 保护
   - 使用 context.Context 传递超时和取消

4. **依赖注入**
   - 使用接口而非具体类型
   - 便于测试和替换实现

### TypeScript 代码规范

1. **使用严格模式**
   - `strict: true` 在 tsconfig.json
   - 所有变量必须有类型

2. **代码风格**
   - 使用 ESLint 检查
   - 遵循 @antfu/eslint-config 规范

3. **模块化**
   - 单个文件不超过 300 行
   - 按功能拆分模块

## 测试指南

### Go 测试

```bash
# 运行所有测试
cd server
go test ./...

# 运行特定包的测试
go test ./internal/storage

# 查看覆盖率
go test -cover ./...

# 生成覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### 测试文件位置

```
server/internal/
├── config/
│   └── config_test.go
├── model/
│   └── types_test.go
├── storage/
│   └── redis_test.go
└── crawler/
    └── scheduler_test.go
```

## 提交规范

### 提交消息格式

```
<类型>: <描述>

[可选的详细描述]
```

### 类型标识

| 类型 | 说明 | 示例 |
|------|------|------|
| `feat` | 新功能 | `feat: 添加站点搜索功能` |
| `fix` | 修复 bug | `fix: 修复 Redis 连接泄漏` |
| `refactor` | 重构 | `refactor: 优化存储层接口设计` |
| `docs` | 文档 | `docs: 更新部署文档` |
| `test` | 测试 | `test: 添加存储层单元测试` |
| `chore` | 构建/工具 | `chore: 升级依赖版本` |

### 提交流程

```bash
# 1. 查看变更
git status

# 2. 暂存文件
git add <files>

# 3. 提交
git commit -m "feat: 添加新功能"

# 4. 推送
git push
```

## 问题排查

### 后端问题

**问题：Redis 连接失败**

```bash
# 检查 Redis 是否运行
redis-cli ping

# 检查配置
cat server/.env | grep REDIS
```

**问题：端口已被占用**

```bash
# 查找占用进程
lsof -i :7066

# 停止进程
kill -9 <PID>
```

### 前端问题

**问题：依赖安装失败**

```bash
# 清理缓存
rm -rf node_modules pnpm-lock.yaml
pnpm store prune
pnpm install
```

**问题：构建失败**

```bash
# 检查 TypeScript 错误
pnpm build --debug
```

### Air 热重载问题

```bash
# 检查 Air 配置
cat server/.air.toml

# 手动清理 tmp 目录
rm -rf server/tmp
air
```

## 调试技巧

### Go 调试

```bash
# 使用 delve 调试器
go install github.com/go-delve/delve/cmd/dlv@latest
dlv debug ./cmd/top1000
```

### 查看 Redis 数据

```bash
# 连接 Redis
redis-cli

# 查看所有 key
KEYS *

# 获取 Top1000 数据
GET top1000:data

# 获取过期时间
TTL top1000:data
```

### API 测试

```bash
# 获取 Top1000 数据
curl http://localhost:7066/top1000.json

# 获取站点列表（需要配置 IYUU_SIGN）
curl http://localhost:7066/sites.json

# 查看 Swagger 文档
open http://localhost:7066/swagger/
```

## 相关文档

- [部署文档](./DEPLOYMENT.md)
- [项目根文档](../CLAUDE.md)
- [API 文档](../server/docs/)

## 许可证

MIT License
