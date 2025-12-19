# Eino-mini

一个轻量的 LLM Chat 后端 + 前端演示项目，基于 Eino ChatModel，支持 Redis 会话、两阶段存储、会话级串行锁，以及 SSE 流式输出。

## 功能

- 对话会话存储（Redis list）
- 两阶段写入（user 先落库，assistant 插回 user 后）
- 会话裁剪（按轮次/字符数）
- 会话级串行锁（同一会话并发排队/限流）
- SSE 流式输出（/ask/stream）
- 纯前端页面（可直接打开或用静态服务器）

## 启动

1) 准备配置

```bash
cp .env.example .env
```

编辑 `.env`，至少配置：

- `OPENAI_API_KEY`
- `OPENAI_BASE_URL`
- `OPENAI_MODEL`
- `REDIS_PASSWORD`

2) 启动 Redis（可选：用 docker-compose）

```bash
docker compose up -d
```

3) 启动后端

```bash
go run .
```

4) 启动前端

```bash
cd frontend
python3 -m http.server 5173
```

打开浏览器访问：
`http://localhost:5173`

> 前端默认请求 `http://localhost:8080`，如需修改可在页面里注入 `window.API_BASE`。

## API

### POST /ask

请求：

```json
{
  "conversation_id": "可选",
  "question": "你好"
}
```

响应：

```json
{
  "conversation_id": "xxx",
  "answer": "..."
}
```

### POST /ask/stream (SSE)

请求同 `/ask`，响应为 SSE 流：

```
event: meta
data: {"conversation_id":"..."}

event: delta
data: {"delta":"..."}

event: done
data: {"answer":"...","conversation_id":"..."}
```

错误事件：

```
event: error
data: {"error":"..."}
```

## 配置项（.env）

- `PORT`：HTTP 端口（默认 8080）
- `OPENAI_API_KEY` / `OPENAI_BASE_URL` / `OPENAI_MODEL`
- `REDIS_ADDR` / `REDIS_PASSWORD` / `REDIS_DB`
- `CHAT_SESSION_TTL`：会话 TTL
- `CHAT_MAX_TURNS` / `CHAT_MAX_CHARS`：裁剪策略
- `CHAT_LOCK_TTL` / `CHAT_LOCK_WAIT`：会话锁配置
- `CHAT_SYSTEM_PROMPT`：默认 system prompt

## 目录结构

- `internal/httpapi`：HTTP API
- `internal/llm`：LLM 客户端
- `internal/session`：会话与 Redis 存储
- `frontend`：前端页面
