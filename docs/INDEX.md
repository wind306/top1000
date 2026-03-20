# 文档索引

Top1000 项目文档导航。

**最后更新:** 2026-01-30

## 项目文档

| 文档 | 说明 | 位置 |
|------|------|------|
| [项目根文档](../CLAUDE.md) | 项目概览、架构、快速启动 | `/CLAUDE.md` |
| [部署文档](./DEPLOYMENT.md) | 部署步骤、Docker 配置 | `/DEPLOYMENT.md` |

## 开发文档

| 文档 | 说明 | 位置 |
|------|------|------|
| [开发工作流](./CONTRIB.md) | 开发环境、代码规范、提交规范 | `/docs/CONTRIB.md` |
| [脚本命令参考](./REFERENCE.md) | 所有可用的脚本命令 | `/docs/REFERENCE.md` |
| [环境变量配置](./ENV.md) | 环境变量详细说明 | `/docs/ENV.md` |

## 运维文档

| 文档 | 说明 | 位置 |
|------|------|------|
| [运维手册](./RUNBOOK.md) | 运维操作、故障处理、监控 | `/docs/RUNBOOK.md` |

## 模块文档

| 模块 | 说明 | 位置 |
|------|------|------|
| 后端入口 | Go 应用入口 | `/server/cmd/top1000/CLAUDE.md` |
| API 处理器 | HTTP API | `/server/internal/api/CLAUDE.md` |
| 爬虫模块 | 数据采集 | `/server/internal/crawler/CLAUDE.md` |
| 存储层 | Redis 存储 | `/server/internal/storage/CLAUDE.md` |
| 配置模块 | 配置管理 | `/server/internal/config/CLAUDE.md` |
| 数据模型 | 数据结构 | `/server/internal/model/CLAUDE.md` |
| 服务器 | Fiber 配置 | `/server/internal/server/CLAUDE.md` |
| 前端应用 | TypeScript + Vite | `/web/CLAUDE.md` |

## 快速链接

- **新用户**: 从 [项目根文档](../CLAUDE.md) 开始
- **开发者**: 参考 [开发工作流](./CONTRIB.md)
- **运维人员**: 参考 [运维手册](./RUNBOOK.md)
- **部署**: 参考 [部署文档](./DEPLOYMENT.md)

## 文档维护

- 所有文档包含"最后更新"时间戳
- 过时文档应更新或删除
- 新功能需同步更新相关文档

## 外部资源

- [Go 1.26 文档](https://go.dev/doc/)
- [Fiber 框架](https://docs.gofiber.io/)
- [Redis 文档](https://redis.io/docs/)
- [Vite 文档](https://vitejs.dev/)
- [AG Grid 文档](https://www.ag-grid.com/)
