package audio

import (
	"fmt"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/sirupsen/logrus"
)

// RecorderNew 音频录制器，直接使用PortAudio进行录音
type RecorderNew struct {
	stream          *portaudio.Stream     // PortAudio流
	buffer          []int16               // 录音缓冲区
	mutex           sync.Mutex            // 互斥锁，保护状态变量
	isRecording     bool                  // 是否正在录音
	onAudioData     func([]int16, int)    // 原始PCM数据回调
	onEncodedData   func([]byte)          // 编码后数据回调
	sampleRate      int                   // 采样率
	channelCount    int                   // 通道数
	framesPerBuffer int                   // 每次回调的帧数
	device          *portaudio.DeviceInfo // 录音设备
	encoder         Encoder               // 编码器（可选）
}

// NewRecorderOptions 创建录音器的选项
type NewRecorderOptions struct {
	SampleRate       int
	ChannelCount     int
	FramesPerBuffer  int
	UseDefaultDevice bool
	DeviceName       string // 如果不为空，则尝试使用指定名称的设备
}

// NewRecorderWithOptions 使用指定选项创建新的录音器
func NewRecorderWithOptions(options NewRecorderOptions) (*RecorderNew, error) {
	if err := portaudio.Initialize(); err != nil {
		return nil, fmt.Errorf("初始化PortAudio失败: %v", err)
	}

	// 使用默认值处理未指定的选项
	if options.SampleRate <= 0 {
		options.SampleRate = DefaultSampleRate
	}
	if options.ChannelCount <= 0 {
		options.ChannelCount = DefaultChannelCount
	}
	if options.FramesPerBuffer <= 0 {
		options.FramesPerBuffer = DefaultFramesPerBuffer
	}

	// 创建录音器
	recorder := &RecorderNew{
		buffer:          make([]int16, options.FramesPerBuffer*options.ChannelCount),
		sampleRate:      options.SampleRate,
		channelCount:    options.ChannelCount,
		framesPerBuffer: options.FramesPerBuffer,
	}

	// 如果指定了设备名称，尝试查找该设备
	if !options.UseDefaultDevice && options.DeviceName != "" {
		devices, err := portaudio.Devices()
		if err != nil {
			return nil, fmt.Errorf("获取音频设备列表失败: %v", err)
		}

		found := false
		for _, dev := range devices {
			if dev.MaxInputChannels > 0 && (dev.Name == options.DeviceName ||
				(len(options.DeviceName) > 3 &&
					dev.Name != "" &&
					contains(dev.Name, options.DeviceName))) {
				recorder.device = dev
				found = true
				logrus.Infof("使用指定的输入设备: %s", dev.Name)
				break
			}
		}

		if !found && options.DeviceName != "" {
			logrus.Warnf("未找到指定的输入设备: %s，将使用默认设备", options.DeviceName)
		}
	}

	// 打印可用设备信息
	if recorder.device == nil && logrus.GetLevel() >= logrus.DebugLevel {
		devices, _ := portaudio.Devices()
		logrus.Debug("可用的输入设备:")
		for i, dev := range devices {
			if dev.MaxInputChannels > 0 {
				logrus.Debugf("[%d] %s (通道数: %d, 默认采样率: %.0f)",
					i, dev.Name, dev.MaxInputChannels, dev.DefaultSampleRate)
			}
		}
	}

	return recorder, nil
}

// NewRecorder 创建新的录音器（使用默认选项）
func NewRecorder(sampleRate, channelCount, framesPerBuffer int) *RecorderNew {
	options := NewRecorderOptions{
		SampleRate:       sampleRate,
		ChannelCount:     channelCount,
		FramesPerBuffer:  framesPerBuffer,
		UseDefaultDevice: true,
	}

	recorder, err := NewRecorderWithOptions(options)
	if err != nil {
		logrus.Errorf("创建录音器失败: %v, 将返回非功能性实例", err)
		// 返回一个非功能性实例，避免nil检查
		return &RecorderNew{
			sampleRate:      sampleRate,
			channelCount:    channelCount,
			framesPerBuffer: framesPerBuffer,
		}
	}

	return recorder
}

// SetPCMDataCallback 设置PCM数据回调函数
func (r *RecorderNew) SetPCMDataCallback(callback func([]int16, int)) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.onAudioData = callback
}

// SetAudioDataCallback 设置编码后的音频数据回调函数
func (r *RecorderNew) SetAudioDataCallback(callback func([]byte)) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.onEncodedData = callback
}

// StartRecording 开始录音
func (r *RecorderNew) StartRecording(encoder Encoder) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.isRecording {
		return fmt.Errorf("录音已经开始")
	}

	// 存储编码器
	r.encoder = encoder

	// 配置输入参数
	var inputParams portaudio.StreamParameters
	if r.device != nil {
		// 使用指定的设备
		inputParams = portaudio.StreamParameters{
			Input: portaudio.StreamDeviceParameters{
				Device:   r.device,
				Channels: r.channelCount,
				Latency:  r.device.DefaultLowInputLatency,
			},
			SampleRate:      float64(r.sampleRate),
			FramesPerBuffer: r.framesPerBuffer,
		}
	} else {
		// 获取默认主机API
		api, err := portaudio.DefaultHostApi()
		if err == nil && api.DefaultInputDevice != nil {
			// 使用当前主机API的默认输入设备
			r.device = api.DefaultInputDevice
			logrus.Infof("使用默认输入设备: %s", r.device.Name)

			inputParams = portaudio.StreamParameters{
				Input: portaudio.StreamDeviceParameters{
					Device:   r.device,
					Channels: r.channelCount,
					Latency:  r.device.DefaultLowInputLatency,
				},
				SampleRate:      float64(r.sampleRate),
				FramesPerBuffer: r.framesPerBuffer,
			}
		} else {
			// 退化方式：不指定具体设备，让PortAudio选择
			inputParams = portaudio.StreamParameters{
				Input: portaudio.StreamDeviceParameters{
					Device:   nil,
					Channels: r.channelCount,
					Latency:  0,
				},
				SampleRate:      float64(r.sampleRate),
				FramesPerBuffer: r.framesPerBuffer,
			}
		}
	}

	// 录音回调函数
	callback := func(in, _ []int16) {
		// 使用延迟恢复机制防止崩溃
		defer func() {
			if rec := recover(); rec != nil {
				logrus.Errorf("录音回调发生异常: %v", rec)
			}
		}()

		// 复制输入数据到缓冲区
		copy(r.buffer, in)

		// 如果设置了原始PCM回调，则调用它
		if r.onAudioData != nil {
			// 深拷贝以避免数据竞争
			pcmData := make([]int16, len(in))
			copy(pcmData, in)
			r.onAudioData(pcmData, len(pcmData))
		}

		// 如果设置了编码后数据回调，且有编码器，则编码并回调
		if r.onEncodedData != nil && r.encoder != nil {
			// 编码数据
			encodedData, err := r.encoder.Encode(r.buffer)
			if err != nil {
				logrus.Errorf("编码音频数据失败: %v", err)
				return
			}
			r.onEncodedData(encodedData)
		}
	}

	// 打开音频流
	stream, err := portaudio.OpenStream(inputParams, callback)
	if err != nil {
		logrus.Errorf("打开音频流失败，尝试PulseAudio: %v", err)

		// 尝试查找PulseAudio设备
		devices, _ := portaudio.Devices()
		var pulseDevice *portaudio.DeviceInfo

		for _, dev := range devices {
			if dev.MaxInputChannels > 0 && contains(dev.Name, "pulse") {
				pulseDevice = dev
				logrus.Infof("找到PulseAudio输入设备: %s", dev.Name)
				break
			}
		}

		if pulseDevice != nil {
			// 使用PulseAudio设备重试
			inputParams.Input.Device = pulseDevice
			r.device = pulseDevice

			stream, err = portaudio.OpenStream(inputParams, callback)
			if err != nil {
				return fmt.Errorf("使用PulseAudio打开音频流失败: %v", err)
			}
		} else {
			return fmt.Errorf("打开音频流失败，且未找到PulseAudio设备: %v", err)
		}
	}

	// 启动音频流
	if err := stream.Start(); err != nil {
		stream.Close()
		return fmt.Errorf("启动音频流失败: %v", err)
	}

	r.stream = stream
	r.isRecording = true
	logrus.Info("录音已开始")

	return nil
}

// StopRecording 停止录音
func (r *RecorderNew) StopRecording() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.isRecording || r.stream == nil {
		return nil
	}

	// 停止和关闭流
	err := r.stopStreamSafely()

	r.stream = nil
	r.isRecording = false
	logrus.Info("录音已停止")

	return err
}

// 安全地停止音频流，处理超时情况
func (r *RecorderNew) stopStreamSafely() error {
	// 创建一个通道来接收结果
	done := make(chan error, 1)

	// 在一个独立的goroutine中执行停止操作
	go func() {
		if r.stream == nil {
			done <- nil
			return
		}

		// 先停止流
		err := r.stream.Stop()
		if err != nil {
			done <- fmt.Errorf("停止音频流失败: %v", err)
			return
		}

		// 关闭流
		err = r.stream.Close()
		if err != nil {
			done <- fmt.Errorf("关闭音频流失败: %v", err)
			return
		}

		done <- nil
	}()

	// 等待操作完成或超时
	select {
	case err := <-done:
		return err
	case <-time.After(1 * time.Second):
		logrus.Warn("停止音频流操作超时")
		return fmt.Errorf("停止音频流操作超时")
	}
}

// IsRecording 检查是否正在录音
func (r *RecorderNew) IsRecording() bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.isRecording
}

// Close 关闭录音器并释放资源
func (r *RecorderNew) Close() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// 添加恢复机制
	defer func() {
		if rec := recover(); rec != nil {
			logrus.Errorf("关闭录音器时发生异常: %v", rec)
		}
	}()

	// 如果正在录音，先停止
	if r.isRecording && r.stream != nil {
		_ = r.stopStreamSafely()
		r.stream = nil
		r.isRecording = false
	}

	// 清除回调和编码器引用
	r.onAudioData = nil
	r.onEncodedData = nil
	r.encoder = nil

	return nil
}

// 工具函数：检查字符串是否包含子串（不区分大小写）
func contains(s, substr string) bool {
	s, substr = toLower(s), toLower(substr)
	return s != "" && substr != "" && indexOf(s, substr) >= 0
}

// 转换为小写
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

// 查找子串位置
func indexOf(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	if len(substr) > len(s) {
		return -1
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
