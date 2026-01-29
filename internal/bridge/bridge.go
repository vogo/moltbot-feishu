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
	feishuCli := feishu.NewClient(cfg.FeishuAppID, cfg.FeishuAppSecret, cfg.ThinkingThresholdMs)
	moltbotCli := moltbot.NewClient(cfg.GatewayPort, cfg.GatewayToken, cfg.MoltbotAgentID)

	return &Bridge{
		cfg:        cfg,
		feishuCli:  feishuCli,
		moltbotCli: moltbotCli,
	}
}

func (b *Bridge) Run(ctx context.Context) error {
	// 连接 Moltbot Gateway
	log.Printf("正在连接 Moltbot Gateway (ws://127.0.0.1:%d)...", b.cfg.GatewayPort)
	if err := b.moltbotCli.Connect(ctx); err != nil {
		return fmt.Errorf("连接 Moltbot Gateway 失败: %w", err)
	}
	log.Println("已连接 Moltbot Gateway")

	// 设置消息处理器
	b.feishuCli.SetHandler(b.handleMessage)

	// 启动飞书客户端
	log.Println("正在启动飞书桥接...")
	return b.feishuCli.Start(ctx)
}

func (b *Bridge) handleMessage(ctx context.Context, chatID, text string) (string, error) {
	sessionKey := fmt.Sprintf("feishu:%s", chatID)

	log.Printf("收到消息: chatID=%s, text=%s", chatID, truncate(text, 50))

	// 发送消息到 Moltbot
	runID, deltaCh, errCh, err := b.moltbotCli.SendMessage(ctx, sessionKey, text)
	if err != nil {
		return "", fmt.Errorf("发送到 Moltbot 失败: %w", err)
	}

	log.Printf("Moltbot 开始处理: runID=%s", runID)

	// 收集流式响应
	var reply strings.Builder
	timeout := time.After(5 * time.Minute)

	for {
		select {
		case delta, ok := <-deltaCh:
			if !ok {
				// 流结束
				result := strings.TrimSpace(reply.String())
				log.Printf("Moltbot 回复完成: %s", truncate(result, 100))
				return result, nil
			}
			reply.WriteString(delta)
		case err := <-errCh:
			return "", err
		case <-timeout:
			return "", fmt.Errorf("等待 Moltbot 响应超时")
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
