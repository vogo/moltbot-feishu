package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

const (
	SeenTTL = 10 * time.Minute
)

type MessageHandler func(ctx context.Context, chatID, text string) (string, error)

type Client struct {
	appID     string
	appSecret string
	larkCli   *lark.Client

	handler MessageHandler

	// 去重
	seenMsgs    map[string]time.Time
	seenMsgLock sync.Mutex

	thinkingThresholdMs int
}

type TextContent struct {
	Text string `json:"text"`
}

func NewClient(appID, appSecret string, thinkingMs int) *Client {
	cli := lark.NewClient(appID, appSecret,
		lark.WithLogLevel(larkcore.LogLevelInfo),
	)

	return &Client{
		appID:               appID,
		appSecret:           appSecret,
		larkCli:             cli,
		seenMsgs:            make(map[string]time.Time),
		thinkingThresholdMs: thinkingMs,
	}
}

func (c *Client) SetHandler(handler MessageHandler) {
	c.handler = handler
}

func (c *Client) Start(ctx context.Context) error {
	// 创建事件分发器 (verificationToken 和 encryptKey 在 WebSocket 模式下可为空)
	eventDispatcher := dispatcher.NewEventDispatcher("", "")

	// 注册消息事件处理器
	eventDispatcher.OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
		return c.handleMessage(ctx, event)
	})

	// 注意: SDK 没有 Stop 方法, 依赖 context 取消来退出
	// 禁用 AutoReconnect 以便 context 取消时能快速退出
	wsClient := larkws.NewClient(c.appID, c.appSecret,
		larkws.WithEventHandler(eventDispatcher),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	log.Println("正在连接飞书 WebSocket...")
	return wsClient.Start(ctx)
}

func (c *Client) Close() {
	// 飞书 SDK 没有 Stop 方法, 依赖 context 取消
}

func (c *Client) handleMessage(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	if event.Event == nil || event.Event.Message == nil {
		return nil
	}

	msg := event.Event.Message
	msgID := *msg.MessageId

	// 去重检查
	if c.isDuplicate(msgID) {
		return nil
	}

	// 只处理文本消息
	if msg.MessageType == nil || *msg.MessageType != "text" {
		return nil
	}

	// 解析消息内容
	var content TextContent
	if err := json.Unmarshal([]byte(*msg.Content), &content); err != nil {
		log.Printf("解析消息内容失败: %v", err)
		return nil
	}

	text := content.Text
	if text == "" {
		return nil
	}

	chatID := *msg.ChatId
	chatType := ""
	if msg.ChatType != nil {
		chatType = *msg.ChatType
	}

	// 群聊智能过滤
	if chatType == "group" {
		if !c.shouldRespondInGroup(text, event.Event.Message.Mentions) {
			return nil
		}
	}

	// 移除 @ 提及
	text = c.stripMentions(text)
	if text == "" {
		return nil
	}

	// 异步处理消息
	go c.processMessage(ctx, chatID, text)

	return nil
}

func (c *Client) isDuplicate(msgID string) bool {
	c.seenMsgLock.Lock()
	defer c.seenMsgLock.Unlock()

	now := time.Now()

	// 清理过期条目
	for id, t := range c.seenMsgs {
		if now.Sub(t) > SeenTTL {
			delete(c.seenMsgs, id)
		}
	}

	if _, exists := c.seenMsgs[msgID]; exists {
		return true
	}
	c.seenMsgs[msgID] = now
	return false
}

func (c *Client) shouldRespondInGroup(text string, mentions []*larkim.MentionEvent) bool {
	// 被 @ 了
	if len(mentions) > 0 {
		return true
	}

	// 以问号结尾
	text = strings.TrimSpace(text)
	if strings.HasSuffix(text, "?") || strings.HasSuffix(text, "？") {
		return true
	}

	lowerText := strings.ToLower(text)

	// 英文疑问词
	questionWords := []string{"why", "how", "what", "when", "where", "who", "help"}
	for _, word := range questionWords {
		if strings.Contains(lowerText, word) {
			return true
		}
	}

	// 中文请求动词
	chineseVerbs := []string{"帮", "麻烦", "请", "能否", "可以", "解释", "看看", "排查", "分析", "总结", "写", "改", "修", "查", "对比", "翻译"}
	for _, verb := range chineseVerbs {
		if strings.Contains(text, verb) {
			return true
		}
	}

	// 以机器人名称开头
	botNames := []string{"alen", "moltbot", "bot", "助手", "智能体"}
	for _, name := range botNames {
		if strings.HasPrefix(lowerText, name) {
			return true
		}
	}

	return false
}

func (c *Client) stripMentions(text string) string {
	// 移除 @_user_xxx 格式的提及
	re := regexp.MustCompile(`@_user_\d+\s*`)
	text = re.ReplaceAllString(text, "")
	return strings.TrimFunc(text, unicode.IsSpace)
}

func (c *Client) processMessage(ctx context.Context, chatID, text string) {
	if c.handler == nil {
		log.Println("未设置消息处理器")
		return
	}

	var placeholderID string
	var done bool
	var mu sync.Mutex

	// 思考中提示定时器
	var timer *time.Timer
	if c.thinkingThresholdMs > 0 {
		timer = time.AfterFunc(time.Duration(c.thinkingThresholdMs)*time.Millisecond, func() {
			mu.Lock()
			defer mu.Unlock()
			if done {
				return
			}
			id, err := c.sendMessage(ctx, chatID, "正在思考…")
			if err == nil {
				placeholderID = id
			}
		})
	}

	// 调用处理器获取回复
	reply, err := c.handler(ctx, chatID, text)

	// 停止定时器
	if timer != nil {
		timer.Stop()
	}

	mu.Lock()
	done = true
	currentPlaceholder := placeholderID
	mu.Unlock()

	if err != nil {
		log.Printf("处理消息失败: %v", err)
		reply = fmt.Sprintf("处理消息时发生错误: %v", err)
	}

	// 跳过空回复或 NO_REPLY
	if reply == "" || reply == "NO_REPLY" {
		if currentPlaceholder != "" {
			c.deleteMessage(ctx, currentPlaceholder)
		}
		return
	}

	// 发送或更新回复
	if currentPlaceholder != "" {
		if err := c.updateMessage(ctx, currentPlaceholder, reply); err != nil {
			// 更新失败,尝试发送新消息
			c.sendMessage(ctx, chatID, reply)
		}
	} else {
		c.sendMessage(ctx, chatID, reply)
	}
}

func (c *Client) sendMessage(ctx context.Context, chatID, text string) (string, error) {
	content, _ := json.Marshal(TextContent{Text: text})

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeText).
			Content(string(content)).
			Build()).
		Build()

	resp, err := c.larkCli.Im.V1.Message.Create(ctx, req)
	if err != nil {
		return "", err
	}
	if !resp.Success() {
		return "", fmt.Errorf("发送消息失败: %s", resp.Msg)
	}

	if resp.Data != nil && resp.Data.MessageId != nil {
		return *resp.Data.MessageId, nil
	}
	return "", nil
}

func (c *Client) updateMessage(ctx context.Context, msgID, text string) error {
	content, _ := json.Marshal(TextContent{Text: text})

	req := larkim.NewPatchMessageReqBuilder().
		MessageId(msgID).
		Body(larkim.NewPatchMessageReqBodyBuilder().
			Content(string(content)).
			Build()).
		Build()

	resp, err := c.larkCli.Im.V1.Message.Patch(ctx, req)
	if err != nil {
		return err
	}
	if !resp.Success() {
		return fmt.Errorf("更新消息失败: %s", resp.Msg)
	}
	return nil
}

func (c *Client) deleteMessage(ctx context.Context, msgID string) error {
	req := larkim.NewDeleteMessageReqBuilder().
		MessageId(msgID).
		Build()

	resp, err := c.larkCli.Im.V1.Message.Delete(ctx, req)
	if err != nil {
		return err
	}
	if !resp.Success() {
		return fmt.Errorf("删除消息失败: %s", resp.Msg)
	}
	return nil
}
