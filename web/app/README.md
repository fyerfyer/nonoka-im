# Nonoka IM Web App

官方 Web 客户端，基于 Next.js 16 App Router + TypeScript + Tailwind CSS + shadcn/ui 构建。

## 技术栈

| 维度 | 选择 |
|------|------|
| 框架 | Next.js 16 (App Router) |
| 语言 | TypeScript |
| 样式 | Tailwind CSS 4 |
| 组件库 | shadcn/ui + @base-ui/react |
| 状态管理 | Zustand |
| 实时通信 | 原生 WebSocket + protobufjs |
| 字体 | Inter (next/font/google) + 中文系统字体回退 |

## 目录结构

```text
web/app/
├── app/                        # Next.js 路由
│   ├── (auth)/                 # 认证路由组
│   │   ├── login/page.tsx      # 登录页
│   │   └── register/page.tsx   # 注册页
│   ├── chat/
│   │   ├── page.tsx            # 聊天主页（桌面双栏 / 移动端会话列表）
│   │   └── [topic]/page.tsx    # 聊天详情页
│   ├── layout.tsx              # 根布局（字体、AuthInitializer）
│   └── globals.css             # 全局样式与主题变量
├── components/
│   ├── auth/                   # 登录/注册表单
│   ├── chat/                   # 聊天相关组件
│   │   ├── ChatLayout.tsx
│   │   ├── ChatHeader.tsx
│   │   ├── ConversationList.tsx
│   │   ├── ConversationItem.tsx
│   │   ├── MessageList.tsx
│   │   ├── MessageBubble.tsx
│   │   ├── MessageInput.tsx
│   │   ├── NewChatDialog.tsx
│   │   ├── NewGroupDialog.tsx
│   │   └── ConnectionBar.tsx
│   └── ui/                     # shadcn/ui 组件
├── hooks/
│   ├── useAuth.ts
│   ├── useRealtime.ts          # WebSocket 生命周期
│   ├── useConversation.ts      # 单会话消息拉取/发送/已读
│   └── useConversations.ts     # 会话列表刷新
├── lib/
│   ├── api.ts                  # HTTP API 封装
│   ├── realtime.ts             # RealtimeClient（WS + Protobuf）
│   ├── topic.ts                # P2P / Group Topic 生成与解析
│   ├── uuid.ts                 # client_msg_id 生成
│   └── proto/                  # protobufjs 生成的代码
├── stores/
│   ├── authStore.ts            # 登录态
│   └── chatStore.ts            # 会话/消息/群信息
└── types/
    └── index.ts                # TypeScript 类型
```

## 开发

### 安装依赖

```bash
cd web/app
pnpm install
```

### 启动开发服务器

```bash
pnpm dev
```

默认监听 [http://localhost:3000](http://localhost:3000)。

### 后端地址

前端默认连接本机后端：

- HTTP API: `http://127.0.0.1:8000/v1`
- WebSocket: `ws://127.0.0.1:8000/ws`

可在 `lib/api.ts` 与 `lib/realtime.ts` 中修改。

### 生成 Protobuf 代码

当后端 `api/im/v1/*.proto` 发生变更时：

```bash
pnpm proto:gen
```

### 代码格式化

```bash
pnpm format
```

## 已完成功能

- [x] 用户注册 / 登录 / Token 持久化
- [x] WebSocket 认证、心跳、断线重连
- [x] 单聊（P2P）实时消息收发与历史消息拉取
- [x] 群聊创建、用户搜索、群消息广播
- [x] 会话列表（最近消息预览、未读数、群名称）
- [x] 消息状态：sending / sent / delivered / read
- [x] 响应式布局（桌面双栏 / 移动端单栏）

## 已知限制

- 当前仅支持文本消息
- 消息搜索、文件/图片、语音通话、消息撤回等暂未实现
- 暗黑模式已预留 CSS 变量，UI 层面未完整适配
