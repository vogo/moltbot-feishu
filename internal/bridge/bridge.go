package bridge

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/vogo/moltbot-feishu/internal/config"
	"github.com/vogo/moltbot-feishu/internal/feishu"
	"github.com/vogo/moltbot-feishu/internal/moltbot"
)

type Bridge struct {
	cfg        *config.Config
	feishuCli  *feishu.Client
	moltbotCli *moltbot.Client
}

func New(cfg *config.Config) *Bridge {
	feishuCli := feishu.NewClient(cfg.FeishuAppID, cfg.FeishuAppSecret)
	moltbotCli := moltbot.NewClient(cfg.GatewayPort, cfg.GatewayToken, cfg.MoltbotAgentID)

	return &Bridge{
		cfg:        cfg,
		feishuCli:  feishuCli,
		moltbotCli: moltbotCli,
	}
}

func (b *Bridge) Run(ctx context.Context) error {
	// 连接 Moltbot Gateway (10秒超时)
	log.Printf("正在连接 Moltbot Gateway (ws://127.0.0.1:%d)...", b.cfg.GatewayPort)
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := b.moltbotCli.Connect(connectCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("连接 Moltbot Gateway 失败: %w", err)
	}
	log.Println("已连接 Moltbot Gateway")

	// 确保退出时关闭连接
	defer b.Close()

	// 设置消息处理器
	b.feishuCli.SetHandler(b.handleMessage)

	// 启动飞书客户端
	log.Println("正在启动飞书桥接...")
	return b.feishuCli.Start(ctx)
}

func (b *Bridge) Close() {
	log.Println("正在关闭连接...")
	b.feishuCli.Close()
	b.moltbotCli.Close()
}

func (b *Bridge) handleMessage(ctx context.Context, chatID, text string, reply func(string) error) error {
	sessionKey := fmt.Sprintf("feishu:%s", chatID)

	log.Printf("收到消息: chatID=%s, text=%s", chatID, truncate(text, 50))

	// 发送消息到 Moltbot
	runID, deltaCh, errCh, err := b.moltbotCli.SendMessage(ctx, sessionKey, text)
	if err != nil {
		return fmt.Errorf("发送到 Moltbot 失败: %w", err)
	}

	log.Printf("Moltbot 开始处理: runID=%s", runID)

	var accumulated strings.Builder
	globalTimeout := time.After(5 * time.Minute)
	idleTimer := time.NewTimer(2 * time.Second)
	idleTimer.Stop() // 初始停止，收到第一个 delta 后启动

	// 发送累积的消息
	sendAccumulated := func() {
		content := strings.TrimSpace(accumulated.String())
		if content != "" {
			if err := reply(content); err != nil {
				log.Printf("发送回复失败: %v", err)
			}
			accumulated.Reset()
		}
	}

	for {
		select {
		case delta, ok := <-deltaCh:
			if !ok {
				// 流结束，发送剩余内容
				idleTimer.Stop()
				sendAccumulated()
				log.Printf("Moltbot 回复完成")
				return nil
			}
			// 累积内容
			accumulated.WriteString(delta)
			// 重置 5 秒定时器
			idleTimer.Stop()
			idleTimer = time.NewTimer(2 * time.Second)

		case <-idleTimer.C:
			// 5 秒没有新 delta，发送累积的内容
			sendAccumulated()

		case err := <-errCh:
			idleTimer.Stop()
			return err

		case <-globalTimeout:
			idleTimer.Stop()
			sendAccumulated()
			return fmt.Errorf("等待 Moltbot 响应超时")

		case <-ctx.Done():
			idleTimer.Stop()
			return ctx.Err()
		}
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
