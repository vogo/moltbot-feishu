package moltbot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	ProtocolVersion = 3
	ClientVersion   = "0.2.0"
)

type Client struct {
	gatewayURL   string
	gatewayToken string
	agentID      string

	conn     *websocket.Conn
	connLock sync.Mutex

	pendingReqs map[string]chan *Response
	reqLock     sync.Mutex

	eventHandlers map[string]func(payload json.RawMessage)
	handlerLock   sync.RWMutex
}

type Request struct {
	Type   string      `json:"type"`
	ID     string      `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params,omitempty"`
}

type Response struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	OK      bool            `json:"ok"`
	Event   string          `json:"event,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *ErrorPayload   `json:"error,omitempty"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ConnectParams struct {
	MinProtocol int        `json:"minProtocol"`
	MaxProtocol int        `json:"maxProtocol"`
	Client      ClientInfo `json:"client"`
	Role        string     `json:"role"`
	Scopes      []string   `json:"scopes"`
	Auth        AuthInfo   `json:"auth"`
	Locale      string     `json:"locale"`
	UserAgent   string     `json:"userAgent"`
}

type ClientInfo struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Mode     string `json:"mode"`
}

type AuthInfo struct {
	Token string `json:"token"`
}

type AgentParams struct {
	Message        string `json:"message"`
	AgentID        string `json:"agentId"`
	SessionKey     string `json:"sessionKey"`
	Deliver        bool   `json:"deliver"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type AgentResponse struct {
	RunID string `json:"runId"`
}

type AgentEvent struct {
	RunID  string          `json:"runId"`
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}

type AssistantDelta struct {
	Delta string `json:"delta"`
}

type LifecycleData struct {
	Phase string `json:"phase"`
}

func NewClient(port int, token, agentID string) *Client {
	return &Client{
		gatewayURL:    fmt.Sprintf("ws://127.0.0.1:%d", port),
		gatewayToken:  token,
		agentID:       agentID,
		pendingReqs:   make(map[string]chan *Response),
		eventHandlers: make(map[string]func(payload json.RawMessage)),
	}
}

func (c *Client) Connect(ctx context.Context) error {
	log.Printf("[Moltbot] 开始连接 Gateway: %s", c.gatewayURL)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	log.Printf("[Moltbot] 正在建立 WebSocket 连接...")
	conn, _, err := dialer.DialContext(ctx, c.gatewayURL, nil)
	if err != nil {
		log.Printf("[Moltbot] WebSocket 连接失败: %v", err)
		return fmt.Errorf("连接 Gateway 失败: %w", err)
	}

	// 只在设置 conn 时加锁
	c.connLock.Lock()
	c.conn = conn
	c.connLock.Unlock()
	log.Printf("[Moltbot] WebSocket 连接已建立")

	// 启动消息读取协程
	go c.readLoop()

	// 等待 connect.challenge
	log.Printf("[Moltbot] 等待 Gateway 握手 (connect.challenge)...")
	challengeCh := make(chan struct{}, 1)
	c.OnEvent("connect.challenge", func(_ json.RawMessage) {
		challengeCh <- struct{}{}
	})

	select {
	case <-challengeCh:
		log.Printf("[Moltbot] 收到握手请求")
	case <-time.After(5 * time.Second):
		log.Printf("[Moltbot] 握手超时 (5秒)")
		conn.Close()
		return fmt.Errorf("等待 Gateway 握手超时")
	case <-ctx.Done():
		log.Printf("[Moltbot] 连接被取消")
		conn.Close()
		return ctx.Err()
	}

	// 发送认证请求
	log.Printf("[Moltbot] 发送认证请求 (protocol=%d, role=operator)...", ProtocolVersion)
	platform := runtime.GOOS
	params := ConnectParams{
		MinProtocol: ProtocolVersion,
		MaxProtocol: ProtocolVersion,
		Client: ClientInfo{
			ID:       "gateway-client",
			Version:  ClientVersion,
			Platform: platform,
			Mode:     "backend",
		},
		Role:      "operator",
		Scopes:    []string{"operator.read", "operator.write"},
		Auth:      AuthInfo{Token: c.gatewayToken},
		Locale:    "zh-CN",
		UserAgent: "moltbot-feishu-bridge-go",
	}

	resp, err := c.sendRequest(ctx, "connect", "connect", params)
	if err != nil {
		log.Printf("[Moltbot] 认证请求失败: %v", err)
		conn.Close()
		return fmt.Errorf("认证失败: %w", err)
	}
	if !resp.OK {
		conn.Close()
		errMsg := "未知错误"
		if resp.Error != nil {
			errMsg = resp.Error.Message
		}
		log.Printf("[Moltbot] 认证被拒绝: %s", errMsg)
		return fmt.Errorf("认证被拒绝: %s", errMsg)
	}

	log.Printf("[Moltbot] 认证成功, 连接就绪")
	return nil
}

func (c *Client) Close() error {
	c.connLock.Lock()
	defer c.connLock.Unlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) OnEvent(event string, handler func(payload json.RawMessage)) {
	c.handlerLock.Lock()
	defer c.handlerLock.Unlock()
	c.eventHandlers[event] = handler
}

func (c *Client) SendMessage(ctx context.Context, sessionKey, message string) (string, <-chan string, <-chan error, error) {
	params := AgentParams{
		Message:        message,
		AgentID:        c.agentID,
		SessionKey:     sessionKey,
		Deliver:        false,
		IdempotencyKey: uuid.New().String(),
	}

	resp, err := c.sendRequest(ctx, "agent", "agent", params)
	if err != nil {
		return "", nil, nil, err
	}
	if !resp.OK {
		errMsg := "请求失败"
		if resp.Error != nil {
			errMsg = resp.Error.Message
		}
		return "", nil, nil, fmt.Errorf("agent 请求失败: %s", errMsg)
	}

	var agentResp AgentResponse
	if err := json.Unmarshal(resp.Payload, &agentResp); err != nil {
		return "", nil, nil, fmt.Errorf("解析响应失败: %w", err)
	}

	// 创建流式响应通道
	deltaCh := make(chan string, 100)
	errCh := make(chan error, 1)

	// 注册事件处理器
	c.OnEvent("agent", func(payload json.RawMessage) {
		var evt AgentEvent
		if err := json.Unmarshal(payload, &evt); err != nil {
			return
		}
		if evt.RunID != agentResp.RunID {
			return
		}

		switch evt.Stream {
		case "assistant":
			var delta AssistantDelta
			if err := json.Unmarshal(evt.Data, &delta); err == nil && delta.Delta != "" {
				select {
				case deltaCh <- delta.Delta:
				default:
				}
			}
		case "lifecycle":
			var lc LifecycleData
			if err := json.Unmarshal(evt.Data, &lc); err == nil && lc.Phase == "end" {
				close(deltaCh)
			}
		}
	})

	return agentResp.RunID, deltaCh, errCh, nil
}

func (c *Client) sendRequest(ctx context.Context, id, method string, params interface{}) (*Response, error) {
	req := Request{
		Type:   "req",
		ID:     id,
		Method: method,
		Params: params,
	}

	respCh := make(chan *Response, 1)
	c.reqLock.Lock()
	c.pendingReqs[id] = respCh
	c.reqLock.Unlock()

	defer func() {
		c.reqLock.Lock()
		delete(c.pendingReqs, id)
		c.reqLock.Unlock()
	}()

	c.connLock.Lock()
	err := c.conn.WriteJSON(req)
	c.connLock.Unlock()
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}

	select {
	case resp := <-respCh:
		return resp, nil
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("请求超时")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Client) readLoop() {
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		var resp Response
		if err := json.Unmarshal(data, &resp); err != nil {
			continue
		}

		switch resp.Type {
		case "res":
			c.reqLock.Lock()
			if ch, ok := c.pendingReqs[resp.ID]; ok {
				ch <- &resp
			}
			c.reqLock.Unlock()
		case "event":
			c.handlerLock.RLock()
			if handler, ok := c.eventHandlers[resp.Event]; ok {
				go handler(resp.Payload)
			}
			c.handlerLock.RUnlock()
		}
	}
}
