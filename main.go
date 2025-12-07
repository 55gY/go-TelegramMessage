package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/telegram/updates"
	updhook "github.com/gotd/td/telegram/updates/hook"
	"github.com/gotd/td/tg"
	"golang.org/x/net/proxy"
	"gopkg.in/yaml.v3"
)

// 配置结构体
type Config struct {
	API struct {
		ApiID       int    `yaml:"api_id"`
		ApiHash     string `yaml:"api_hash"`
		SessionFile string `yaml:"session_file"`
		ProxyAddr   string `yaml:"proxy_addr"`
	} `yaml:"api"`
	
	SubscriptionAPI struct {
		Host   string `yaml:"host"`
		ApiKey string `yaml:"api_key"`
	} `yaml:"subscription_api"`
	
	Features struct {
		FetchHistoryEnabled bool `yaml:"fetch_history_enabled"`
	} `yaml:"features"`
	
	Monitor struct {
		Channels          []int64 `yaml:"channels"`
		WhitelistChannels []int64 `yaml:"whitelist_channels"`
	} `yaml:"monitor"`
	
	Filters struct {
		Keywords      []string `yaml:"keywords"`
		ContentFilter []string `yaml:"content_filter"`
		LinkBlacklist []string `yaml:"link_blacklist"`
	} `yaml:"filters"`
}

// 全局配置变量
var config Config

// 加载配置文件
func loadConfig(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}
	
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}
	
	return nil
}

// 兼容性：保留旧的变量名，从配置中读取
var (
	ApiID       int
	ApiHash     string
	SessionFile string
	ProxyAddr   string
	
	SubscriptionAPIHost string
	SubscriptionAPIKey  string
	
	FetchHistoryEnabled bool
	
	Keywords         []string
	ContentFilter    []string
	LinkBlacklist    []string
	MonitorChannels  []int64
	WhitelistChannels []int64
)

// 初始化配置变量
func initConfigVars() {
	ApiID = config.API.ApiID
	ApiHash = config.API.ApiHash
	SessionFile = config.API.SessionFile
	ProxyAddr = config.API.ProxyAddr
	
	SubscriptionAPIHost = config.SubscriptionAPI.Host
	SubscriptionAPIKey = config.SubscriptionAPI.ApiKey
	
	FetchHistoryEnabled = config.Features.FetchHistoryEnabled
	
	Keywords = config.Filters.Keywords
	ContentFilter = config.Filters.ContentFilter
	LinkBlacklist = config.Filters.LinkBlacklist
	
	MonitorChannels = config.Monitor.Channels
	WhitelistChannels = config.Monitor.WhitelistChannels
}

func main() {
	// 加载配置文件
	if err := loadConfig("config.yaml"); err != nil {
		fmt.Printf("❌ 配置文件加载失败: %v\n", err)
		fmt.Println("💡 提示: 请确保 config.yaml 文件存在")
		return
	}
	
	// 初始化配置变量
	initConfigVars()
	
	fmt.Println("✅ 配置文件加载成功")
	fmt.Printf("📝 监听 %d 个频道\n", len(MonitorChannels))
	fmt.Printf("📝 关键词数量: %d\n", len(Keywords))
	fmt.Printf("📝 白名单频道数量: %d\n", len(WhitelistChannels))
	fmt.Println()
	
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("\n❌ 程序崩溃: %v\n", r)
		}
	}()

	fmt.Println("🚀 程序启动...")
	fmt.Printf("📱 API ID: %d\n", ApiID)
	fmt.Printf("🔑 API Hash: %s...\n", ApiHash[:10])
	fmt.Printf("💾 会话文件: %s\n\n", SessionFile)

	if ApiID == 0 || ApiHash == "" {
		fmt.Println("❌ 错误: 请先配置 API ID 和 API Hash")
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	fmt.Printf("📡 Context 状态: %v\n\n", ctx.Err())

	// 配置代理
	proxyURL, err := url.Parse("socks5://" + ProxyAddr)
	if err != nil {
		fmt.Printf("❌ 代理地址解析失败: %v\n", err)
		return
	}

	dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
	if err != nil {
		fmt.Printf("❌ 代理配置失败: %v\n", err)
		return
	}

	fmt.Printf("🔌 使用代理: %s\n\n", ProxyAddr)

	// 创建 Telegram 客户端
	fmt.Println("🔧 创建 Telegram 客户端...")

	// 先创建 dispatcher 和 gaps (按照官方示例)
	dispatcher := tg.NewUpdateDispatcher()
	var updateCount int64
	var dispatchCount int64

	// 添加一个包装器来计数和调试
	rawHandler := telegram.UpdateHandlerFunc(func(ctx context.Context, u tg.UpdatesClass) error {
		updateCount++

		// 只在有消息相关的更新时才打印
		hasMessage := false
		switch update := u.(type) {
		case *tg.Updates:
			for _, upd := range update.Updates {
				switch upd.(type) {
				case *tg.UpdateNewMessage, *tg.UpdateNewChannelMessage, *tg.UpdateEditMessage, *tg.UpdateEditChannelMessage:
					hasMessage = true
					dispatchCount++
				}
			}
		case *tg.UpdateShortMessage, *tg.UpdateShortChatMessage:
			hasMessage = true
			dispatchCount++
		}

		// 只有包含消息时才打印
		if hasMessage {
			fmt.Printf("\n[%s] 收到消息更新 (#%d)\n", time.Now().Format("15:04:05"), updateCount)
		}

		// 传递给 dispatcher 处理
		err := dispatcher.Handle(ctx, u)
		if err != nil && hasMessage {
			fmt.Printf("  ⚠️ 处理错误: %v\n", err)
		}
		return err
	})

	gaps := updates.New(updates.Config{
		Handler: rawHandler,
	})

	// 注册消息处理器
	dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewMessage) error {
		msg, ok := update.Message.(*tg.Message)
		if !ok {
			return nil
		}
		return handleMessage(msg, e)
	})

	dispatcher.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewChannelMessage) error {
		msg, ok := update.Message.(*tg.Message)
		if !ok {
			return nil
		}
		return handleMessage(msg, e)
	})

	// 添加编辑消息处理器
	dispatcher.OnEditMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateEditMessage) error {
		if msg, ok := update.Message.(*tg.Message); ok {
			return handleMessage(msg, e)
		}
		return nil
	})

	dispatcher.OnEditChannelMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateEditChannelMessage) error {
		if msg, ok := update.Message.(*tg.Message); ok {
			return handleMessage(msg, e)
		}
		return nil
	})

	// 使用带信号监听的原始 ctx,不添加超时限制
	var dialCount int
	client := telegram.NewClient(ApiID, ApiHash, telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{Path: SessionFile},
		DialTimeout:    30 * time.Second, // 每个连接30秒超时
		UpdateHandler:  gaps,             // 设置 gaps 为更新处理器
		Middlewares: []telegram.Middleware{
			updhook.UpdateHook(gaps.Handle), // 关键：添加 UpdateHook 中间件
		},
		Resolver: dcs.Plain(dcs.PlainOptions{
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				dialCount++
				fmt.Printf("🔗 [#%d] 正在连接: %s %s\n", dialCount, network, address)

				// 为每个连接设置30秒超时
				dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
				defer dialCancel()

				conn, err := dialer.(proxy.ContextDialer).DialContext(dialCtx, network, address)
				if err != nil {
					fmt.Printf("❌ [#%d] 连接失败: %v\n", dialCount, err)
				} else {
					fmt.Printf("✅ [#%d] 连接成功: %s\n", dialCount, address)
				}
				return conn, err
			},
		}),
	})

	// 运行客户端
	fmt.Println("🔌 连接到 Telegram 服务器...")
	fmt.Println("⏰ 开始执行 client.Run...")
	fmt.Println("💡 提示: 如果长时间卡在连接,可以:")
	fmt.Println("   1. 删除 session.json 重新登录")
	fmt.Println("   2. 检查代理是否稳定")
	fmt.Println("   3. 尝试禁用 IPv6")
	fmt.Println()

	// 添加一个 goroutine 监控连接进度
	progressDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		startTime := time.Now()
		lastDialCount := 0
		noProgressCount := 0
		for {
			select {
			case <-progressDone:
				return
			case <-ticker.C:
				elapsed := time.Since(startTime).Round(time.Second)
				fmt.Printf("⏳ [%s] 等待回调中... (已用时: %v, 连接次数: %d)\n",
					time.Now().Format("15:04:05"), elapsed, dialCount)

				// 检测是否有进展
				if dialCount == lastDialCount {
					noProgressCount++
					if noProgressCount >= 6 { // 30秒无进展
						fmt.Println("⚠️ 30秒无进展,建议:")
						fmt.Println("   - 按 Ctrl+C 停止程序")
						fmt.Println("   - 删除 session.json 文件")
						fmt.Println("   - 重新运行程序")
					}
				} else {
					noProgressCount = 0
				}
				lastDialCount = dialCount
			}
		}
	}()

	runErr := client.Run(ctx, func(ctx context.Context) error {
		close(progressDone) // 停止进度监控
		fmt.Printf("\n✨ [%s] 回调函数被调用！\n", time.Now().Format("15:04:05"))
		fmt.Println("🔐 开始认证流程...")
		// 登录
		if err := authenticate(ctx, client); err != nil {
			fmt.Printf("❌ 认证失败: %v\n", err)
			return err
		}

		fmt.Println("✅ 登录成功！")

		// 获取当前用户信息
		api := client.API()
		self, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUserSelf{}})
		if err != nil {
			fmt.Printf("❌ 获取用户信息失败: %v\n", err)
			return err
		}

		user := self[0].(*tg.User)
		fmt.Printf("👤 当前用户: %s %s (ID: %d)\n", user.FirstName, user.LastName, user.ID)
		fmt.Printf("📋 监听关键词: %v\n", Keywords)
		if len(MonitorChannels) > 0 {
			fmt.Printf("🎯 监听频道: %v\n", MonitorChannels)
		} else {
			fmt.Println("🌐 监听所有频道")
		}
		fmt.Println()

		// 获取对话列表来验证连接
		fmt.Println("📝 获取对话列表...")
		dialogs, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetDate: 0,
			OffsetID:   0,
			OffsetPeer: &tg.InputPeerEmpty{},
			Limit:      10,
			Hash:       0,
		})
		if err != nil {
			fmt.Printf("⚠️ 获取对话列表失败: %v\n", err)
		} else {
			switch d := dialogs.(type) {
			case *tg.MessagesDialogs:
				fmt.Printf("✅ 找到 %d 个对话\n", len(d.Dialogs))
			case *tg.MessagesDialogsSlice:
				fmt.Printf("✅ 找到 %d 个对话 (总共 %d 个)\n", len(d.Dialogs), d.Count)
			}
		}
		fmt.Println()

		// 获取指定频道的历史消息（可通过 FetchHistoryEnabled 开关控制）
		if FetchHistoryEnabled && len(MonitorChannels) > 0 {
			fmt.Println("📜 开始获取历史消息...")
			for _, channelID := range MonitorChannels {
				if err := fetchChannelHistory(ctx, api, channelID); err != nil {
					fmt.Printf("⚠️ 获取频道 %d 历史消息失败: %v\n", channelID, err)
				}
			}
			fmt.Println("✅ 历史消息获取完成")
			fmt.Println()
		}

		// 启动监听
		fmt.Println("👂 开始监听实时消息...")
		fmt.Println("⏳ 等待新消息中...")
		fmt.Println("💡 提示: 程序会显示已加入的频道/群组的新消息")
		fmt.Println("📌 注意: 可能会先收到最近的几条历史消息,然后等待新消息")
		fmt.Println("🔄 测试方法: 向任何已加入的频道/群组发送消息,或等待其他人发送")
		fmt.Println()

		// 启动心跳检测
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			startTime := time.Now()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					uptime := time.Since(startTime).Round(time.Second)
					fmt.Printf("[%s] 运行:%v | 消息:%d\n",
						time.Now().Format("15:04:05"), uptime, dispatchCount)
				}
			}
		}()

		// 使用正确的用户ID - 按照官方示例运行 gaps.Run
		fmt.Printf("\n🚀 启动 gaps.Run (UserID: %d, IsBot: %v)\n", user.ID, user.Bot)

		// 按照官方示例的方式运行 gaps
		return gaps.Run(ctx, api, user.ID, updates.AuthOptions{
			IsBot: user.Bot,
			OnStart: func(ctx context.Context) {
				fmt.Println("✅ Gaps started - 开始接收实时更新")
			},
		})
	})

	fmt.Printf("🏁 client.Run 完成，错误: %v\n", runErr)
	if runErr != nil {
		fmt.Printf("❌ 详细错误: %v\n", runErr)
		return
	}

	fmt.Println("\n👋 程序正常退出")
}

// handleMessage 处理消息并检查关键词
func handleMessage(msg *tg.Message, e tg.Entities) error {
	messageText := msg.Message

	// ✅ 频道过滤检查
	var channelID int64
	if msg.PeerID != nil {
		if peer, ok := msg.PeerID.(*tg.PeerChannel); ok {
			channelID = peer.ChannelID
		}
	}

	// 如果配置了监听频道列表,则只处理这些频道的消息
	if len(MonitorChannels) > 0 {
		allowedChannel := false
		for _, id := range MonitorChannels {
			if id == channelID {
				allowedChannel = true
				break
			}
		}
		// 不在监听列表中的频道,直接跳过
		if !allowedChannel {
			return nil
		}
	}

	// ✅ 启用关键词匹配功能
	matched := false
	for _, keyword := range Keywords {
		if strings.Contains(strings.ToLower(messageText), strings.ToLower(keyword)) {
			matched = true
			break
		}
	}

	// 如果没有匹配关键词,直接跳过
	if !matched {
		return nil
	}

	// ✅ 检查是否在白名单中
	isWhitelisted := false
	for _, whiteID := range WhitelistChannels {
		if whiteID == channelID {
			isWhitelisted = true
			break
		}
	}

	// 如果不在白名单中,需要进行二次过滤
	if !isWhitelisted {
		// ✅ 消息内容二次过滤 - 检查是否包含“投稿”或“订阅”
		contentMatched := false
		for _, filterWord := range ContentFilter {
			if strings.Contains(messageText, filterWord) {
				contentMatched = true
				break
			}
		}

		// 如果消息不包含指定关键字,直接跳过
		if !contentMatched {
			return nil
		}
	}
	// 提取消息中的链接
	links := extractLinks(messageText)

	// 只显示提取到的链接
	if len(links) > 0 {
		// 获取来源类型
		var source string
		if msg.PeerID != nil {
			switch peer := msg.PeerID.(type) {
			case *tg.PeerChannel:
				source = fmt.Sprintf("频道:%d", peer.ChannelID)
			case *tg.PeerChat:
				source = fmt.Sprintf("群组:%d", peer.ChatID)
			case *tg.PeerUser:
				source = fmt.Sprintf("私聊:%d", peer.UserID)
			}
		}

		// 单行显示: [时间] 来源 | 链接
		for _, link := range links {
			fmt.Printf("[%s] %s | %s\n",
				time.Now().Format("15:04:05"),
				source,
				link)

			// 🔥 自动添加订阅链接
			success, message := addSubscription(link)
			if success {
				fmt.Printf("  ✅ 订阅添加成功: %s\n", message)
			} else {
				if message == "订阅已存在" {
					fmt.Printf("  ⚠️  订阅已存在，跳过\n")
				} else {
					fmt.Printf("  ❌ 订阅添加失败: %s\n", message)
				}
			}
		}
	}

	return nil
} // 认证登录
func authenticate(ctx context.Context, client *telegram.Client) error {
	return client.Auth().IfNecessary(
		ctx,
		auth.NewFlow(
			&terminalAuth{},
			auth.SendCodeOptions{},
		),
	)
}

// terminalAuth 终端认证器
type terminalAuth struct{}

func (terminalAuth) Phone(_ context.Context) (string, error) {
	fmt.Print("请输入手机号（国际格式，如 +8613800138000）: ")
	phone, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		fmt.Printf("❌ 读取手机号失败: %v\n", err)
		return "", err
	}
	phone = strings.TrimSpace(phone)
	fmt.Printf("📞 使用手机号: %s\n", phone)
	return phone, nil
}

func (terminalAuth) Password(_ context.Context) (string, error) {
	fmt.Print("请输入密码（如果启用了两步验证）: ")
	pwd, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		fmt.Printf("❌ 读取密码失败: %v\n", err)
		return "", err
	}
	return strings.TrimSpace(pwd), nil
}

func (terminalAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	fmt.Print("请输入收到的验证码: ")
	code, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(code), err
}

func (terminalAuth) AcceptTermsOfService(_ context.Context, tos tg.HelpTermsOfService) error {
	return nil
}

func (terminalAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("需要注册")
}

// extractLinks 从文本中提取所有链接，并过滤黑名单关键字
func extractLinks(text string) []string {
	var links []string
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 查找包含 http:// 或 https:// 的行
		if strings.Contains(line, "http://") || strings.Contains(line, "https://") {
			// 循环提取当前行中的所有链接
			remainingLine := line
			for len(remainingLine) > 0 {
				// 查找 http:// 或 https:// 的位置
				httpIdx := strings.Index(remainingLine, "http://")
				httpsIdx := strings.Index(remainingLine, "https://")

				startIdx := -1
				if httpIdx >= 0 && httpsIdx >= 0 {
					startIdx = min(httpIdx, httpsIdx)
				} else if httpIdx >= 0 {
					startIdx = httpIdx
				} else if httpsIdx >= 0 {
					startIdx = httpsIdx
				}

				// 如果没有找到链接，退出循环
				if startIdx < 0 {
					break
				}

				// 从 http/https 开始提取，直到遇到空格、换行或其他分隔符
				linkStart := startIdx
				linkEnd := linkStart
				for linkEnd < len(remainingLine) {
					ch := remainingLine[linkEnd]
					// 遇到空格、换行、中文符号等结束
					if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
						break
					}
					linkEnd++
				}

				link := remainingLine[linkStart:linkEnd]
				// 清理可能的尾部标点符号（包括中文和英文标点）
				link = strings.TrimRight(link, ",.;!?，。；！？、")

				// 检查链接是否包含黑名单关键字
				isBlacklisted := false
				linkLower := strings.ToLower(link)
				for _, blackword := range LinkBlacklist {
					if strings.Contains(linkLower, strings.ToLower(blackword)) {
						isBlacklisted = true
						break
					}
				}

				// 只添加不在黑名单中的链接
				if !isBlacklisted && len(link) > 8 { // 至少要有 https:// 的长度
					links = append(links, link)
				}

				// 继续处理剩余部分
				remainingLine = remainingLine[linkEnd:]
			}
		}
	}
	return links
}

// min 返回两个整数中较小的一个
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// fetchChannelHistory 获取指定频道的历史消息
func fetchChannelHistory(ctx context.Context, api *tg.Client, channelID int64) error {
	fmt.Printf("\n📥 正在获取频道 %d 的历史消息...\n", channelID)

	// 构造 InputPeerChannel
	inputPeer := &tg.InputPeerChannel{
		ChannelID:  channelID,
		AccessHash: 0, // 通常需要从之前的请求中获取
	}

	// 尝试通过 InputChannel 获取消息
	// 如果 AccessHash 未知，尝试先解析频道
	channel, err := api.ChannelsGetChannels(ctx, []tg.InputChannelClass{
		&tg.InputChannel{
			ChannelID:  channelID,
			AccessHash: 0,
		},
	})

	if err != nil {
		// 如果失败，尝试从对话中查找 AccessHash
		dialogs, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetDate: 0,
			OffsetID:   0,
			OffsetPeer: &tg.InputPeerEmpty{},
			Limit:      100,
			Hash:       0,
		})

		if err != nil {
			return fmt.Errorf("获取对话列表失败: %w", err)
		}

		// 查找对应的频道
		var accessHash int64
		var foundChannel *tg.Channel
		switch d := dialogs.(type) {
		case *tg.MessagesDialogs:
			for _, chat := range d.Chats {
				if ch, ok := chat.(*tg.Channel); ok && ch.ID == channelID {
					accessHash = ch.AccessHash
					foundChannel = ch
					break
				}
			}
		case *tg.MessagesDialogsSlice:
			for _, chat := range d.Chats {
				if ch, ok := chat.(*tg.Channel); ok && ch.ID == channelID {
					accessHash = ch.AccessHash
					foundChannel = ch
					break
				}
			}
		}

		if foundChannel == nil {
			return fmt.Errorf("未找到频道 %d，请确认已加入该频道", channelID)
		}

		fmt.Printf("📢 频道名称: %s\n", foundChannel.Title)
		inputPeer.AccessHash = accessHash
	} else {
		// 成功获取频道信息
		switch chats := channel.(type) {
		case *tg.MessagesChats:
			if len(chats.Chats) > 0 {
				if ch, ok := chats.Chats[0].(*tg.Channel); ok {
					fmt.Printf("📢 频道名称: %s\n", ch.Title)
					inputPeer.AccessHash = ch.AccessHash
				}
			}
		}
	}

	// 获取历史消息
	history, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:       inputPeer,
		OffsetID:   0,
		OffsetDate: 0,
		AddOffset:  0,
		Limit:      100, // 获取最近100条
		MaxID:      0,
		MinID:      0,
		Hash:       0,
	})

	if err != nil {
		return fmt.Errorf("获取历史消息失败: %w", err)
	}

	// 处理历史消息
	var messages []tg.MessageClass
	var users map[int64]*tg.User
	var channels map[int64]*tg.Channel

	switch h := history.(type) {
	case *tg.MessagesMessages:
		messages = h.Messages
		users = make(map[int64]*tg.User)
		for _, u := range h.Users {
			if user, ok := u.(*tg.User); ok {
				users[user.ID] = user
			}
		}
		channels = make(map[int64]*tg.Channel)
		for _, c := range h.Chats {
			if channel, ok := c.(*tg.Channel); ok {
				channels[channel.ID] = channel
			}
		}
	case *tg.MessagesMessagesSlice:
		messages = h.Messages
		users = make(map[int64]*tg.User)
		for _, u := range h.Users {
			if user, ok := u.(*tg.User); ok {
				users[user.ID] = user
			}
		}
		channels = make(map[int64]*tg.Channel)
		for _, c := range h.Chats {
			if channel, ok := c.(*tg.Channel); ok {
				channels[channel.ID] = channel
			}
		}
	case *tg.MessagesChannelMessages:
		messages = h.Messages
		users = make(map[int64]*tg.User)
		for _, u := range h.Users {
			if user, ok := u.(*tg.User); ok {
				users[user.ID] = user
			}
		}
		channels = make(map[int64]*tg.Channel)
		for _, c := range h.Chats {
			if channel, ok := c.(*tg.Channel); ok {
				channels[channel.ID] = channel
			}
		}
	}

	fmt.Printf("📊 获取到 %d 条历史消息\n", len(messages))

	// 处理每条消息
	matchCount := 0
	for i := len(messages) - 1; i >= 0; i-- { // 倒序处理，从旧到新
		msg, ok := messages[i].(*tg.Message)
		if !ok {
			continue
		}

		messageText := msg.Message
		if messageText == "" {
			continue
		}

		// 关键词匹配
		matched := false
		for _, keyword := range Keywords {
			if strings.Contains(strings.ToLower(messageText), strings.ToLower(keyword)) {
				matched = true
				break
			}
		}

		if !matched {
			continue
		}

		// ✅ 检查是否在白名单中
		isWhitelisted := false
		for _, whiteID := range WhitelistChannels {
			if whiteID == channelID {
				isWhitelisted = true
				break
			}
		}

		// 如果不在白名单中,需要进行二次过滤
		if !isWhitelisted {
			// ✅ 消息内容二次过滤 - 检查是否包含“投稿”或“订阅”
			contentMatched := false
			for _, filterWord := range ContentFilter {
				if strings.Contains(messageText, filterWord) {
					contentMatched = true
					break
				}
			}

			if !contentMatched {
				continue
			}
		}

		// 提取消息中的链接
		links := extractLinks(messageText)

		// 只显示提取到的链接
		if len(links) > 0 {
			// 格式化时间
			msgTime := time.Unix(int64(msg.Date), 0).Format("2006-01-02 15:04:05")

			// 输出匹配的链接
			for _, link := range links {
				fmt.Printf("[%s] 频道:%d | %s\n",
					msgTime,
					channelID,
					link)

				// 🔥 自动添加订阅链接
				success, message := addSubscription(link)
				if success {
					fmt.Printf("  ✅ 订阅添加成功: %s\n", message)
				} else {
					if message == "订阅已存在" {
						fmt.Printf("  ⚠️  订阅已存在，跳过\n")
					} else {
						fmt.Printf("  ❌ 订阅添加失败: %s\n", message)
					}
				}
			}

			matchCount++
		}
	}

	fmt.Printf("✅ 频道 %d: 匹配到 %d 条消息\n", channelID, matchCount)
	return nil
}

// addSubscription 添加订阅链接到订阅管理系统
// 参数: subURL - 订阅链接
// 返回: (成功, 消息)
func addSubscription(subURL string) (bool, string) {
	// 构建请求体
	requestBody := map[string]string{
		"sub_url": subURL,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return false, fmt.Sprintf("JSON 编码失败: %v", err)
	}

	// 创建 HTTP 客户端，设置超时
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// 构建请求
	apiURL := fmt.Sprintf("http://%s/api/config/add", SubscriptionAPIHost)
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return false, fmt.Sprintf("创建请求失败: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", SubscriptionAPIKey)

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("API 请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Sprintf("读取响应失败: %v", err)
	}

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var result struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Sprintf("解析响应失败: %v", err)
	}

	// 检查是否是重复订阅
	if result.Error != "" {
		if strings.Contains(result.Error, "已存在") || strings.Contains(strings.ToLower(result.Error), "already exists") {
			return false, "订阅已存在"
		}
		return false, result.Error
	}

	return true, result.Message
}
