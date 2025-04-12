package protocol

// AudioParams 定义音频参数结构
type AudioParams struct {
	Format        string `json:"format"`         // 音频编码格式，例如"opus"
	SampleRate    int    `json:"sample_rate"`    // 采样率，例如16000
	Channels      int    `json:"channels"`       // 声道数，例如1
	FrameDuration int    `json:"frame_duration"` // 帧时长(毫秒)，例如60
}

// HelloMessage 定义客户端初始hello消息
type HelloMessage struct {
	Type        string      `json:"type"`         // 消息类型，必须为"hello"
	Version     int         `json:"version"`      // 协议版本号
	Transport   string      `json:"transport"`    // 传输方式，必须为"websocket"
	AudioParams AudioParams `json:"audio_params"` // 音频参数
}

// ServerHelloMessage 定义服务器响应的hello消息
type ServerHelloMessage struct {
	Type        string       `json:"type"`                   // 消息类型，必须为"hello"
	Transport   string       `json:"transport"`              // 传输方式，必须为"websocket"
	AudioParams *AudioParams `json:"audio_params,omitempty"` // 可选，服务器音频参数
}

// ListenMessage 定义开始/停止录音的消息
type ListenMessage struct {
	SessionID string `json:"session_id"`     // 会话ID
	Type      string `json:"type"`           // 消息类型，必须为"listen"
	State     string `json:"state"`          // 状态: "start", "stop", "detect"
	Mode      string `json:"mode"`           // 模式: "auto", "manual", "realtime"
	Text      string `json:"text,omitempty"` // 可选，当state为"detect"时，包含检测到的唤醒词
}

// AbortMessage 定义终止消息的结构
type AbortMessage struct {
	SessionID string `json:"session_id"` // 会话ID
	Type      string `json:"type"`       // 消息类型，必须为"abort"
	Reason    string `json:"reason"`     // 原因，例如"wake_word_detected"等
}

// STTMessage 定义语音识别结果消息
type STTMessage struct {
	Type string `json:"type"` // 消息类型，必须为"stt"
	Text string `json:"text"` // 识别到的文本
}

// TTSMessage 定义文本转语音控制消息
type TTSMessage struct {
	Type  string `json:"type"`           // 消息类型，必须为"tts"
	State string `json:"state"`          // 状态: "start", "stop", "sentence_start"
	Text  string `json:"text,omitempty"` // 可选，当state为"sentence_start"时包含要朗读的文本
}

// LLMMessage 定义LLM表情/情感指令消息
type LLMMessage struct {
	Type    string `json:"type"`    // 消息类型，必须为"llm"
	Emotion string `json:"emotion"` // 情感类型，例如"happy"
	Text    string `json:"text"`    // 表情文本，例如emoji "😀"
}

// IoTCommandMessage 定义IoT命令消息
type IoTCommandMessage struct {
	Type     string        `json:"type"`     // 消息类型，必须为"iot"
	Commands []interface{} `json:"commands"` // IoT命令数组
}

// IoTStateMessage 定义IoT状态消息
type IoTStateMessage struct {
	SessionID   string      `json:"session_id"`            // 会话ID
	Type        string      `json:"type"`                  // 消息类型，必须为"iot"
	States      interface{} `json:"states,omitempty"`      // 设备状态信息
	Descriptors interface{} `json:"descriptors,omitempty"` // 设备描述信息
}

// MessageType 从JSON数据中提取消息类型
func MessageType(data []byte) string {
	// 简单查找"type"字段，这不是一个完全可靠的JSON解析
	// 但对于快速判断消息类型足够了
	for i := 0; i < len(data)-8; i++ {
		if data[i] == '"' && data[i+1] == 't' && data[i+2] == 'y' && data[i+3] == 'p' &&
			data[i+4] == 'e' && data[i+5] == '"' && data[i+6] == ':' && data[i+7] == '"' {
			j := i + 8
			for j < len(data) && data[j] != '"' {
				j++
			}
			if j < len(data) {
				return string(data[i+8 : j])
			}
		}
	}
	return ""
}
