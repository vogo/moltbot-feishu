# Moltbot-Feishu 桥接服务

一个 Go 语言实现的 Moltbot (原 Clawdbot) 与飞书消息平台的桥接服务。

## 特性

- **无需公网服务器**: 利用飞书 WebSocket 长连接，无需公网 IP、域名或 HTTPS 证书
- **流式响应**: 支持 AI 回复的流式传输
- **智能群聊过滤**: 在群聊中只响应 @提及 或包含问题/请求的消息
- **思考中提示**: 当 AI 处理时间较长时显示"正在思考..."提示
- **消息去重**: 自动过滤重复投递的消息
- **灵活配置**: 支持命令行参数和环境变量两种配置方式

## 架构

```
┌──────────────┐
│  飞书用户    │
└──────┬───────┘
       │ (消息)
       ▼
┌──────────────────┐
│  飞书云服务      │
└──────┬───────────┘
       │ WebSocket 长连接 (仅出站)
       ▼
┌──────────────────────────────┐
│  moltbot-feishu (本地运行)   │
│  - 接收飞书事件              │
│  - 转发到 Moltbot Gateway    │
│  - 处理 AI 流式响应          │
└──────┬───────────────────────┘
       │ WebSocket (127.0.0.1:18789)
       ▼
┌──────────────────────────────┐
│  Moltbot Gateway             │
│  - AI Agent 调度             │
│  - 会话管理                  │
└──────────────────────────────┘
```

## 安装

### 方式一：下载预编译二进制（推荐）

从 [Releases](https://github.com/vogo/moltbot-feishu/releases) 页面下载对应平台的预编译二进制文件：

| 平台 | 架构 | 文件名 |
|------|------|--------|
| Linux | amd64 | `moltbot-feishu-linux-amd64.tar.gz` |
| Linux | arm64 | `moltbot-feishu-linux-arm64.tar.gz` |
| Linux | 386 | `moltbot-feishu-linux-386.tar.gz` |
| macOS | Intel | `moltbot-feishu-darwin-amd64.tar.gz` |
| macOS | Apple Silicon | `moltbot-feishu-darwin-arm64.tar.gz` |
| Windows | amd64 | `moltbot-feishu-windows-amd64.zip` |
| Windows | arm64 | `moltbot-feishu-windows-arm64.zip` |
| Windows | 386 | `moltbot-feishu-windows-386.zip` |

```bash
# Linux/macOS 示例
wget https://github.com/vogo/moltbot-feishu/releases/latest/download/moltbot-feishu-linux-amd64.tar.gz
tar -xzf moltbot-feishu-linux-amd64.tar.gz
chmod +x moltbot-feishu-linux-amd64
./moltbot-feishu-linux-amd64 --version
```

### 方式二：从源码构建

需要 Go 1.21 或更高版本：

```bash
# 克隆仓库
git clone https://github.com/vogo/moltbot-feishu.git
cd moltbot-feishu

# 下载依赖
go mod tidy

# 构建
go build -o moltbot-feishu .
```

## 前置要求

1. 运行中的 Moltbot Gateway 服务
2. 飞书开放平台应用

## 快速开始

### 1. 创建飞书应用

1. 访问 [飞书开放平台](https://open.feishu.cn/app)
2. 创建企业自建应用
3. 获取 App ID 和 App Secret
4. 配置应用权限：
   - `im:message` - 发送和接收消息
   - `im:message.group_at_msg` - 接收群聊 @消息
   - `im:message.p2p_msg` - 接收私聊消息
5. 启用事件订阅：
   - 订阅方式选择 **WebSocket 长连接**
   - 添加事件: `im.message.receive_v1`
6. 发布应用版本

### 2. 配置 Moltbot

确保 Moltbot Gateway 已运行，配置文件通常位于 `~/.moltbot/moltbot.json`：

```json
{
  "gateway": {
    "port": 18789,
    "auth": "your-gateway-token"
  }
}
```

### 3. 配置

有两种配置方式：

#### 方式一：环境变量

```bash
# 复制示例配置
cp .env.example .env

# 编辑配置
vim .env
```

必需的环境变量：

| 变量 | 说明 |
|------|------|
| `FEISHU_APP_ID` | 飞书应用 App ID |
| `FEISHU_APP_SECRET` | 飞书应用 App Secret |

可选环境变量：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `FEISHU_APP_SECRET_PATH` | `~/.moltbot/secrets/feishu_app_secret` | 密钥文件路径（比环境变量更安全） |
| `MOLTBOT_CONFIG_PATH` | `~/.moltbot/moltbot.json` | Moltbot 配置文件路径 |
| `MOLTBOT_AGENT_ID` | `main` | 使用的 Agent ID |
| `MOLTBOT_GATEWAY_PORT` | `18789` | Gateway 端口 |
| `MOLTBOT_GATEWAY_TOKEN` | - | Gateway 认证 Token |
| `FEISHU_THINKING_THRESHOLD_MS` | `2500` | "正在思考..."提示延迟(毫秒) |

#### 方式二：命令行参数

```bash
./moltbot-feishu \
  --feishu-app-id=your_app_id \
  --feishu-app-secret=your_app_secret \
  --agent-id=main \
  --gateway-port=18789 \
  --gateway-token=your_token
```

完整参数列表：

| 参数 | 说明 |
|------|------|
| `--feishu-app-id` | 飞书应用 App ID |
| `--feishu-app-secret` | 飞书应用 App Secret |
| `--feishu-secret-path` | 飞书密钥文件路径 |
| `--moltbot-config` | Moltbot 配置文件路径 |
| `--agent-id` | Moltbot Agent ID |
| `--gateway-port` | Gateway 端口 |
| `--gateway-token` | Gateway 认证 Token |
| `--thinking-ms` | "正在思考..."提示延迟 |

**配置优先级**: 命令行参数 > 环境变量 > 配置文件 > 默认值

### 4. 运行

```bash
# 使用环境变量
source .env && ./moltbot-feishu

# 或使用命令行参数
./moltbot-feishu --feishu-app-id=xxx --feishu-app-secret=xxx
```

### 5. 测试

在飞书中：
- **私聊**: 直接发送消息给机器人
- **群聊**: @机器人 或发送包含问号的消息

## 安全建议

### 密钥文件存储

推荐使用文件存储敏感密钥：

```bash
# 创建密钥目录
mkdir -p ~/.moltbot/secrets
chmod 700 ~/.moltbot/secrets

# 保存飞书密钥
echo "your_feishu_app_secret" > ~/.moltbot/secrets/feishu_app_secret
chmod 600 ~/.moltbot/secrets/feishu_app_secret
```

然后设置环境变量：
```bash
export FEISHU_APP_SECRET_PATH=~/.moltbot/secrets/feishu_app_secret
```

## 作为系统服务运行

### macOS (launchd)

创建 `~/Library/LaunchAgents/com.moltbot.feishu-bridge.plist`：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.moltbot.feishu-bridge</string>
    <key>ProgramArguments</key>
    <array>
        <string>/path/to/moltbot-feishu</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>FEISHU_APP_ID</key>
        <string>your_app_id</string>
        <key>FEISHU_APP_SECRET_PATH</key>
        <string>/Users/yourname/.moltbot/secrets/feishu_app_secret</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/Users/yourname/.moltbot/logs/feishu-bridge.out.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/yourname/.moltbot/logs/feishu-bridge.err.log</string>
</dict>
</plist>
```

启用服务：
```bash
mkdir -p ~/.moltbot/logs
launchctl load ~/Library/LaunchAgents/com.moltbot.feishu-bridge.plist
```

### Linux (systemd)

创建 `/etc/systemd/system/moltbot-feishu.service`：

```ini
[Unit]
Description=Moltbot Feishu Bridge
After=network.target

[Service]
Type=simple
User=youruser
Environment=FEISHU_APP_ID=your_app_id
Environment=FEISHU_APP_SECRET_PATH=/home/youruser/.moltbot/secrets/feishu_app_secret
ExecStart=/path/to/moltbot-feishu
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

启用服务：
```bash
sudo systemctl daemon-reload
sudo systemctl enable moltbot-feishu
sudo systemctl start moltbot-feishu
```

## 群聊智能过滤

在群聊中，桥接服务只会响应以下类型的消息：

1. **@提及**: 消息中 @了机器人
2. **问号结尾**: 消息以 `?` 或 `？` 结尾
3. **英文疑问词**: 包含 why, how, what, when, where, who, help
4. **中文请求词**: 包含 帮、麻烦、请、能否、可以、解释、看看、排查、分析、总结、写、改、修、查、对比、翻译
5. **机器人名称开头**: 以 alen、moltbot、bot、助手、智能体 开头

## 故障排除

### 连接飞书失败

- 检查 App ID 和 App Secret 是否正确
- 确认应用已发布且权限已开启
- 确认事件订阅方式选择了 WebSocket

### 连接 Gateway 失败

- 确认 Moltbot Gateway 正在运行
- 检查端口号是否正确（默认 18789）
- 验证 Gateway Token 是否正确

### 消息没有回复

- 检查日志确认消息是否被接收
- 群聊中确认消息符合过滤规则
- 确认 Agent ID 配置正确

## 开发

```bash
# 运行测试
go test ./...

# 格式化代码
go fmt ./...

# 静态检查
go vet ./...
```

## 发布新版本

本项目使用 GitHub Actions 自动构建和发布。有两种方式触发发布：

### 方式一：手动触发（推荐）

1. 进入 GitHub 仓库的 **Actions** 页面
2. 选择 **Build and Release** workflow
3. 点击 **Run workflow**
4. 输入版本号（如 `v1.0.0`）
5. 选择是否为预发布版本
6. 点击 **Run workflow** 开始构建

### 方式二：通过 Tag 触发

```bash
git tag v1.0.0
git push origin v1.0.0
```

构建完成后，预编译的二进制文件将自动发布到 Releases 页面。

## 参考

- [原 Node.js 实现](https://github.com/AlexAnys/feishu-moltbot-bridge)
- [飞书开放平台文档](https://open.feishu.cn/document/)
- [飞书 Go SDK](https://github.com/larksuite/oapi-sdk-go)

## 许可证

MIT License
