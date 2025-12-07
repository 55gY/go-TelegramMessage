# go-TelegramMessage

**纯 Go 实现的 Telegram 消息监听器（独立运行）**

[![GitHub](https://img.shields.io/badge/GitHub-55gY%2Fgo--TelegramMessage-blue)](https://github.com/55gY/go-TelegramMessage)

## 📦 项目简介

`go-TelegramMessage` 是一个纯 Go 语言实现的 Telegram 消息监听器，无需依赖任何第三方工具。

### ✨ 核心特性

- ❌ **不依赖 tdl** - 完全独立运行
- 🎯 **轻量级** - 只有约 100 行核心代码
- 🔍 **关键词匹配** - 实时监听并过滤消息
- 🔧 **易于扩展** - 简单清晰的代码结构

## 🔗 相关项目

| 项目 | 说明 | 依赖 tdl | Session 数量 | GitHub |
|------|------|----------|--------------|--------|
| **go-TelegramMessage** (本项目) | 独立消息监听器 | ❌ | 1 | [![GitHub](https://img.shields.io/badge/GitHub-repo-blue)](https://github.com/55gY/go-TelegramMessage) |
| [go-bot](https://github.com/55gY/go-bot) | 独立转发机器人 | ❌ | 1 | [![GitHub](https://img.shields.io/badge/GitHub-repo-blue)](https://github.com/55gY/go-bot) |
| [tdl-msgproce](https://github.com/55gY/tdl-msgproce) | 基于 tdl 的融合版 | ✅ | 1 | [![GitHub](https://img.shields.io/badge/GitHub-repo-blue)](https://github.com/55gY/tdl-msgproce) |

### 📊 项目选择指南

- **需要消息监听且不想安装 tdl**：使用本项目（go-TelegramMessage）
- **需要监听+转发，且已有 tdl**：推荐 [tdl-msgproce](https://github.com/55gY/tdl-msgproce)
- **只需要转发功能**：使用 [go-bot](https://github.com/55gY/go-bot)

## 🚀 快速开始

### 安装

```bash
# 克隆仓库
git clone https://github.com/55gY/go-TelegramMessage.git
cd go-TelegramMessage

# 安装依赖
go get github.com/gotd/td
```

### 配置

编辑源码文件，修改配置：

```go
const (
    ApiID   = 你的API_ID        // 从 https://my.telegram.org 获取
    ApiHash = "你的API_HASH"
)

var Keywords = []string{
    "关键词1",
    "关键词2",
    // 添加更多关键词...
}
```

### 运行

```bash
# 首次运行需要登录
go run main.go

# 按提示输入手机号和验证码
```

### 输出示例

```
✅ 登录成功！
📋 监听关键词: [telegram tdl 下载]

🎯 检测到关键词: telegram
👤 发送者: Zhang San (@zhangsan)
💬 消息: 如何使用telegram下载文件？
---
```

## 📝 核心代码说明

### 消息处理器

```go
// 注册新消息处理器
dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewMessage) error {
    msg, ok := update.Message.(*tg.Message)
    if !ok {
        return nil
    }

    // 获取消息文本
    messageText := msg.Message

    // 关键词匹配
    for _, keyword := range Keywords {
        if strings.Contains(strings.ToLower(messageText), strings.ToLower(keyword)) {
            // 🎯 匹配到关键词！
            fmt.Printf("检测到关键词: %s\n", keyword)
            
            // 在这里添加你的处理逻辑
            // ...
            
            break
        }
    }

    return nil
})
```

## 💡 扩展示例

### 示例 1: 添加自动回复

```go
if strings.Contains(strings.ToLower(messageText), strings.ToLower(keyword)) {
    fmt.Printf("🎯 检测到关键词: %s\n", keyword)
    
    // 发送回复
    sender := message.NewSender(client.API())
    peer := &tg.InputPeerUser{UserID: sender.ID}
    sender.To(peer).Text(ctx, "你好！我看到你提到了相关内容。")
}
```

### 示例 2: 保存到文件

```go
if strings.Contains(strings.ToLower(messageText), strings.ToLower(keyword)) {
    // 保存到文件
    f, _ := os.OpenFile("matched_messages.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    defer f.Close()
    
    log := fmt.Sprintf("[%s] %s: %s\n", 
        time.Now().Format("2006-01-02 15:04:05"),
        sender.Username, 
        messageText)
    f.WriteString(log)
}
```

### 示例 3: 调用 API

```go
if strings.Contains(strings.ToLower(messageText), strings.ToLower(keyword)) {
    // 调用外部 API
    handleKeywordMatch(keyword, sender, messageText)
}

func handleKeywordMatch(keyword string, sender *tg.User, message string) {
    // 发送到订阅系统 API
    // http.Post(...)
}
```

## 📋 代码流程

```
1. 创建 Telegram 客户端
   ↓
2. 登录认证
   ↓
3. 注册消息处理器
   ↓
4. 接收新消息
   ↓
5. 检查关键词匹配
   ↓
6. 执行自定义处理逻辑
```

## 🔧 依赖

只需要一个依赖：

```bash
go get github.com/gotd/td
```

## ⚠️ 注意事项

- 首次运行需要扫码或输入验证码登录
- `session.json` 文件保存登录信息，不要删除
- 不要频繁操作，避免账号被限制
- API ID 和 Hash 从 https://my.telegram.org 获取

## 📄 开源协议

MIT License

## 🔗 相关链接

- **tdl-msgproce**: https://github.com/55gY/tdl-msgproce - 基于 tdl 的融合版（功能更多）
- **go-bot**: https://github.com/55gY/go-bot - 转发机器人
- **gotd/td**: https://github.com/gotd/td - Telegram Go 客户端库

## 💬 支持

遇到问题或有建议？欢迎提交 Issue！

---

💡 **提示**：这是最简化版本，只有约 100 行核心代码！如需更完整的功能，推荐使用 [tdl-msgproce](https://github.com/55gY/tdl-msgproce)。
