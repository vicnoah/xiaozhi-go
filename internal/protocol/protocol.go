package protocol

// Protocol 定义了客户端与服务器通信的基本接口
type Protocol interface {
	// Connect 建立与服务器的连接
	Connect(url string) error

	// Disconnect 断开与服务器的连接
	Disconnect() error

	// SendJSON 发送JSON消息到服务器
	SendJSON(data interface{}) error

	// SendBinary 发送二进制数据到服务器
	SendBinary(data []byte) error

	// SetOnJSONMessage 设置接收JSON消息的回调
	SetOnJSONMessage(callback func(data []byte))

	// SetOnBinaryMessage 设置接收二进制消息的回调
	SetOnBinaryMessage(callback func(data []byte))

	// SetOnDisconnected 设置连接断开的回调
	SetOnDisconnected(callback func(err error))

	// SetOnConnected 设置连接成功的回调
	SetOnConnected(callback func())

	// IsConnected 返回当前连接状态
	IsConnected() bool

	// SetHeader 设置连接请求头
	SetHeader(key, value string)

	// GetHeaders 获取所有设置的请求头
	GetHeaders() map[string]string
}
