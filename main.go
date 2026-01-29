package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/vogo/moltbot-feishu/internal/bridge"
	"github.com/vogo/moltbot-feishu/internal/config"
)

// Version 由构建时注入
var Version = "dev"

func main() {
	// 注册命令行参数
	flags := config.RegisterFlags()
	flag.Parse()

	// 检查版本参数
	if flags.Version {
		fmt.Printf("moltbot-feishu %s\n", Version)
		os.Exit(0)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("Moltbot-Feishu 桥接服务启动中... (版本: %s)", Version)

	// 加载配置
	cfg, err := config.Load(flags)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	log.Printf("配置加载完成: AppID=%s, AgentID=%s, GatewayPort=%d",
		maskSecret(cfg.FeishuAppID), cfg.MoltbotAgentID, cfg.GatewayPort)

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 监听退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("收到信号 %v，正在退出...", sig)
		cancel()
	}()

	// 创建并运行桥接
	b := bridge.New(cfg)
	if err := b.Run(ctx); err != nil {
		if ctx.Err() == nil {
			log.Fatalf("桥接运行失败: %v", err)
		}
	}

	log.Println("服务已停止")
}

func maskSecret(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "****"
}
