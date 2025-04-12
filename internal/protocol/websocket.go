package protocol

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// WebsocketProtocol 实现了Protocol接口，使用WebSocket作为通信方式
type WebsocketProtocol struct {
	conn             *websocket.Conn
	url              string
	mu               sync.Mutex
	connected        bool
	onJSONMessage    func(data []byte)
	onBinaryMessage  func(data []byte)
	onDisconnected   func(err error)
	onConnected      func()
	headers          map[string]string
	readTimeout      time.Duration
	writeTimeout     time.Duration
	handshakeTimeout time.Duration
	skipTLSVerify    bool
	stopChan         chan struct{}
}

// NewWebsocketProtocol 创建一个新的WebSocket协议实例
func NewWebsocketProtocol() *WebsocketProtocol {
	return &WebsocketProtocol{
		headers:          make(map[string]string),
		readTimeout:      30 * time.Second,
		writeTimeout:     30 * time.Second,
		handshakeTimeout: 30 * time.Second,
		skipTLSVerify:    false,
		stopChan:         make(chan struct{}),
	}
}

// SetHeader 设置WebSocket连接的请求头
func (wp *WebsocketProtocol) SetHeader(key, value string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.headers[key] = value
}

// GetHeaders 获取所有设置的请求头
func (wp *WebsocketProtocol) GetHeaders() map[string]string {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	// 返回请求头的副本，避免并发访问问题
	headersCopy := make(map[string]string)
	for k, v := range wp.headers {
		headersCopy[k] = v
	}

	return headersCopy
}

// SetReadTimeout 设置读取超时时间
func (wp *WebsocketProtocol) SetReadTimeout(timeout time.Duration) {
	wp.readTimeout = timeout
}

// SetWriteTimeout 设置写入超时时间
func (wp *WebsocketProtocol) SetWriteTimeout(timeout time.Duration) {
	wp.writeTimeout = timeout
}

// SetHandshakeTimeout 设置握手超时时间
func (wp *WebsocketProtocol) SetHandshakeTimeout(timeout time.Duration) {
	wp.handshakeTimeout = timeout
}

// SetSkipTLSVerify 设置是否跳过TLS证书验证
func (wp *WebsocketProtocol) SetSkipTLSVerify(skip bool) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.skipTLSVerify = skip
}

// Connect 实现Protocol接口，连接到WebSocket服务器
func (wp *WebsocketProtocol) Connect(url string) error {
	wp.mu.Lock()
	if wp.connected {
		wp.mu.Unlock()
		return errors.New("已经连接到服务器")
	}
	wp.url = url
	skipTLSVerify := wp.skipTLSVerify
	wp.mu.Unlock()

	// 准备请求头
	header := make(map[string][]string)
	wp.mu.Lock()
	// 清晰地记录每个请求头
	if len(wp.headers) > 0 {
		logrus.Debug("WebSocket连接请求头:")
		for k, v := range wp.headers {
			header[k] = []string{v}
			logrus.Debugf("  %s: %s", k, v)
		}
	} else {
		logrus.Warn("WebSocket连接没有设置任何请求头")
	}
	wp.mu.Unlock()

	// 尝试解析主机名
	logrus.Debug("准备解析WebSocket服务器地址...")
	parsedURL, err := parseWebSocketURL(url)
	if err != nil {
		logrus.Errorf("解析WebSocket URL失败: %v", err)
		return err
	}

	// 尝试DNS解析
	logrus.Debugf("尝试解析主机名: %s", parsedURL.Hostname)
	ips, err := net.LookupIP(parsedURL.Hostname)
	if err != nil {
		logrus.Errorf("DNS解析失败: %v", err)
		// 我们继续执行，因为Dial函数会再次尝试解析
	} else {
		logrus.Debugf("DNS解析成功，获取到IP地址: %v", ips)
	}

	// 配置拨号器
	dialer := websocket.Dialer{
		HandshakeTimeout: wp.handshakeTimeout,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipTLSVerify,
		},
	}

	logrus.Debugf("开始WebSocket连接: %s", url)
	logrus.Debugf("  跳过TLS验证: %v", skipTLSVerify)
	logrus.Debugf("  握手超时: %v", wp.handshakeTimeout)
	logrus.Debugf("  读取超时: %v", wp.readTimeout)
	logrus.Debugf("  写入超时: %v", wp.writeTimeout)

	// 建立连接
	startTime := time.Now()
	logrus.Debug("正在尝试建立WebSocket连接...")
	conn, resp, err := dialer.Dial(url, header)
	elapsed := time.Since(startTime)

	if err != nil {
		if resp != nil {
			logrus.Errorf("连接WebSocket服务器失败: %v", err)
			logrus.Errorf("HTTP状态码: %d", resp.StatusCode)
			logrus.Errorf("HTTP响应头: %v", resp.Header)
			body := make([]byte, 1024)
			n, readErr := resp.Body.Read(body)
			if readErr != nil && readErr != io.EOF {
				logrus.Errorf("读取响应体失败: %v", readErr)
			} else if n > 0 {
				logrus.Errorf("响应体: %s", string(body[:n]))
			}
			logrus.Errorf("连接用时: %v", elapsed)
		} else {
			logrus.Errorf("连接WebSocket服务器失败: %v", err)
			logrus.Error("无HTTP响应")
			logrus.Errorf("连接用时: %v", elapsed)
		}
		return err
	}

	logrus.Infof("WebSocket连接成功, 用时: %v", elapsed)

	wp.mu.Lock()
	wp.conn = conn
	wp.connected = true
	wp.stopChan = make(chan struct{})
	wp.mu.Unlock()

	// 启动读取循环
	go wp.readPump()

	// 触发连接成功回调
	if wp.onConnected != nil {
		wp.onConnected()
	}

	return nil
}

// 辅助函数，解析WebSocket URL
type ParsedWSURL struct {
	Hostname string
	Port     string
	Path     string
	SSL      bool
}

func parseWebSocketURL(wsURL string) (ParsedWSURL, error) {
	var result ParsedWSURL

	if strings.HasPrefix(wsURL, "wss://") {
		result.SSL = true
		wsURL = strings.TrimPrefix(wsURL, "wss://")
	} else if strings.HasPrefix(wsURL, "ws://") {
		result.SSL = false
		wsURL = strings.TrimPrefix(wsURL, "ws://")
	} else {
		return result, fmt.Errorf("不支持的WebSocket URL格式: %s", wsURL)
	}

	// 查找主机名和路径的分隔符
	parts := strings.SplitN(wsURL, "/", 2)
	hostPort := parts[0]

	// 解析主机名和端口
	hostPortParts := strings.Split(hostPort, ":")
	result.Hostname = hostPortParts[0]

	if len(hostPortParts) > 1 {
		result.Port = hostPortParts[1]
	} else {
		if result.SSL {
			result.Port = "443"
		} else {
			result.Port = "80"
		}
	}

	// 解析路径
	if len(parts) > 1 {
		result.Path = "/" + parts[1]
	} else {
		result.Path = "/"
	}

	return result, nil
}

// Disconnect 实现Protocol接口，断开与WebSocket服务器的连接
func (wp *WebsocketProtocol) Disconnect() error {
	// 快速检查是否已断开，避免后续操作
	wp.mu.Lock()
	if !wp.connected || wp.conn == nil {
		wp.mu.Unlock()
		return nil
	}

	// 立即标记为断开，以便其他代码不再尝试使用此连接
	wp.connected = false
	conn := wp.conn
	wp.conn = nil

	// 尝试关闭停止通道，忽略已关闭的情况
	select {
	case <-wp.stopChan:
		// 通道已关闭，这是正常的
	default:
		close(wp.stopChan)
	}
	wp.mu.Unlock()

	// 启动一个goroutine来关闭连接，完全不阻塞当前操作
	go func() {
		// 捕获所有可能的异常
		defer func() {
			if r := recover(); r != nil {
				logrus.Errorf("关闭WebSocket连接时发生异常: %v", r)
			}
		}()

		if conn != nil {
			// 设置非常短的超时，我们不在乎是否成功发送了关闭消息
			conn.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
			// 尝试发送关闭消息，忽略错误
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			// 直接关闭连接
			conn.Close()
		}
	}()

	// 无需等待，立即返回
	return nil
}

// SendJSON 实现Protocol接口，发送JSON消息
func (wp *WebsocketProtocol) SendJSON(data interface{}) error {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if !wp.connected || wp.conn == nil {
		return errors.New("未连接到服务器")
	}

	wp.conn.SetWriteDeadline(time.Now().Add(wp.writeTimeout))
	return wp.conn.WriteJSON(data)
}

// SendBinary 实现Protocol接口，发送二进制数据
func (wp *WebsocketProtocol) SendBinary(data []byte) error {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if !wp.connected || wp.conn == nil {
		return errors.New("未连接到服务器")
	}

	wp.conn.SetWriteDeadline(time.Now().Add(wp.writeTimeout))
	return wp.conn.WriteMessage(websocket.BinaryMessage, data)
}

// SetOnJSONMessage 实现Protocol接口，设置接收JSON消息的回调
func (wp *WebsocketProtocol) SetOnJSONMessage(callback func(data []byte)) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.onJSONMessage = callback
}

// SetOnBinaryMessage 实现Protocol接口，设置接收二进制消息的回调
func (wp *WebsocketProtocol) SetOnBinaryMessage(callback func(data []byte)) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.onBinaryMessage = callback
}

// SetOnDisconnected 实现Protocol接口，设置连接断开的回调
func (wp *WebsocketProtocol) SetOnDisconnected(callback func(err error)) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.onDisconnected = callback
}

// SetOnConnected 实现Protocol接口，设置连接成功的回调
func (wp *WebsocketProtocol) SetOnConnected(callback func()) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.onConnected = callback
}

// IsConnected 实现Protocol接口，返回当前连接状态
func (wp *WebsocketProtocol) IsConnected() bool {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	return wp.connected
}

// readPump 处理从WebSocket接收的消息
func (wp *WebsocketProtocol) readPump() {
	defer func() {
		wp.mu.Lock()
		isConnected := wp.connected
		wp.mu.Unlock()

		if isConnected {
			wp.handleDisconnect(errors.New("WebSocket读取循环结束"))
		}
	}()

	for {
		select {
		case <-wp.stopChan:
			return
		default:
			// 设置读取超时
			wp.conn.SetReadDeadline(time.Now().Add(wp.readTimeout))

			// 读取消息
			messageType, message, err := wp.conn.ReadMessage()
			if err != nil {
				logrus.Errorf("读取WebSocket消息失败: %v", err)
				return
			}

			// 根据消息类型调用不同的回调
			switch messageType {
			case websocket.TextMessage:
				if wp.onJSONMessage != nil {
					wp.onJSONMessage(message)
				}
			case websocket.BinaryMessage:
				if wp.onBinaryMessage != nil {
					wp.onBinaryMessage(message)
				}
			case websocket.CloseMessage:
				return
			}
		}
	}
}

// handleDisconnect 处理连接断开
func (wp *WebsocketProtocol) handleDisconnect(err error) {
	wp.mu.Lock()
	if !wp.connected {
		wp.mu.Unlock()
		return
	}
	wp.connected = false
	if wp.conn != nil {
		wp.conn.Close()
		wp.conn = nil
	}
	onDisconnected := wp.onDisconnected
	wp.mu.Unlock()

	// 触发断开连接回调
	if onDisconnected != nil {
		onDisconnected(err)
	}
}

// ForceDisconnect 立即强制断开连接，不等待任何网络操作
// 这是为了支持程序快速退出而设计的方法
func (wp *WebsocketProtocol) ForceDisconnect() {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	// 如果已经断开，直接返回
	if !wp.connected || wp.conn == nil {
		return
	}

	// 立即标记为断开状态
	wp.connected = false

	// 强制关闭连接
	if wp.conn != nil {
		wp.conn.Close()
		wp.conn = nil
	}

	// 关闭停止通道，如果尚未关闭
	select {
	case <-wp.stopChan:
		// 已经关闭
	default:
		close(wp.stopChan)
	}

	logrus.Debug("WebSocket连接已强制关闭")
}
