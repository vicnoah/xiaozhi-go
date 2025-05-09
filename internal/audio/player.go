package audio

import (
	"fmt"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/sirupsen/logrus"
)

// AudioPlayerNew 音频播放器，直接使用PortAudio播放
type AudioPlayerNew struct {
	stream          *portaudio.Stream     // PortAudio流
	buffer          []int16               // PCM缓冲区
	mutex           sync.Mutex            // 状态互斥锁
	queue           [][]int16             // PCM数据队列
	queueMutex      sync.Mutex            // 队列互斥锁
	isPlaying       bool                  // 是否正在播放
	stopChan        chan struct{}         // 停止信号通道
	stopChanMutex   sync.Mutex            // 通道关闭互斥锁
	stopChanClosed  bool                  // 通道是否已关闭
	sampleRate      int                   // 采样率
	channelCount    int                   // 通道数
	framesPerBuffer int                   // 每次回调的帧数
	device          *portaudio.DeviceInfo // 播放设备
	dummyMode       bool                  // 哑模式标志
	decoder         Decoder               // 解码器（可选）
}

// NewPlayerOptions 创建播放器的选项
type NewPlayerOptions struct {
	SampleRate       int
	ChannelCount     int
	FramesPerBuffer  int
	UseDefaultDevice bool
	DeviceName       string // 如果不为空，则尝试使用指定名称的设备
}

// NewAudioPlayerWithOptions 使用指定选项创建新的音频播放器
func NewAudioPlayerWithOptions(options NewPlayerOptions, decoder Decoder) (*AudioPlayerNew, error) {
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
		// 根据默认帧持续时间计算帧大小
		options.FramesPerBuffer = (DefaultSampleRate * DefaultFrameDuration) / 1000
	}

	// 创建播放器
	player := &AudioPlayerNew{
		buffer:          make([]int16, options.FramesPerBuffer*options.ChannelCount),
		queue:           make([][]int16, 0, 100), // 预分配适当大小的队列
		stopChan:        make(chan struct{}),
		stopChanMutex:   sync.Mutex{},
		stopChanClosed:  false,
		sampleRate:      options.SampleRate,
		channelCount:    options.ChannelCount,
		framesPerBuffer: options.FramesPerBuffer,
		dummyMode:       false,
		decoder:         decoder,
	}

	// 如果指定了设备名称，尝试查找该设备
	if !options.UseDefaultDevice && options.DeviceName != "" {
		devices, err := portaudio.Devices()
		if err != nil {
			return nil, fmt.Errorf("获取音频设备列表失败: %v", err)
		}

		found := false
		for _, dev := range devices {
			if dev.MaxOutputChannels > 0 && (dev.Name == options.DeviceName ||
				(len(options.DeviceName) > 3 &&
					dev.Name != "" &&
					contains(dev.Name, options.DeviceName))) {
				player.device = dev
				found = true
				logrus.Infof("使用指定的输出设备: %s", dev.Name)
				break
			}
		}

		if !found && options.DeviceName != "" {
			logrus.Warnf("未找到指定的输出设备: %s，将使用默认设备", options.DeviceName)
		}
	}

	// 打印可用设备信息
	if player.device == nil && logrus.GetLevel() >= logrus.DebugLevel {
		devices, _ := portaudio.Devices()
		logrus.Debug("可用的输出设备:")
		for i, dev := range devices {
			if dev.MaxOutputChannels > 0 {
				logrus.Debugf("[%d] %s (通道数: %d, 默认采样率: %.0f)",
					i, dev.Name, dev.MaxOutputChannels, dev.DefaultSampleRate)
			}
		}
	}

	return player, nil
}

// NewAudioPlayer2 创建新的音频播放器（使用默认选项）
func NewAudioPlayer2(sampleRate, channelCount, frameDuration int, decoder Decoder) *AudioPlayerNew {
	// 根据帧持续时间计算帧大小
	framesPerBuffer := (sampleRate * frameDuration) / 1000

	options := NewPlayerOptions{
		SampleRate:       sampleRate,
		ChannelCount:     channelCount,
		FramesPerBuffer:  framesPerBuffer,
		UseDefaultDevice: true,
	}

	player, err := NewAudioPlayerWithOptions(options, decoder)
	if err != nil {
		logrus.Errorf("创建音频播放器失败: %v, 将以哑模式运行", err)
		// 返回一个哑模式实例，避免nil检查
		return &AudioPlayerNew{
			buffer:          make([]int16, framesPerBuffer*channelCount),
			queue:           make([][]int16, 0),
			stopChan:        make(chan struct{}),
			sampleRate:      sampleRate,
			channelCount:    channelCount,
			framesPerBuffer: framesPerBuffer,
			dummyMode:       true,
			decoder:         decoder,
		}
	}

	return player
}

// Start 开始音频播放
func (p *AudioPlayerNew) Start() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// 重建stopChan并重置stopChanClosed，保证多次Start/Stop安全
	p.stopChanMutex.Lock()
	if p.stopChanClosed {
		p.stopChan = make(chan struct{})
		p.stopChanClosed = false
	}
	p.stopChanMutex.Unlock()

	if p.isPlaying {
		return nil
	}

	// 如果已经处于哑模式，直接启动队列处理
	if p.dummyMode {
		p.isPlaying = true
		go p.processQueue()
		return nil
	}

	// 配置输出参数
	var outputParams portaudio.StreamParameters
	if p.device != nil {
		// 使用指定的设备
		outputParams = portaudio.StreamParameters{
			Output: portaudio.StreamDeviceParameters{
				Device:   p.device,
				Channels: p.channelCount,
				Latency:  p.device.DefaultLowOutputLatency,
			},
			SampleRate:      float64(p.sampleRate),
			FramesPerBuffer: p.framesPerBuffer,
		}
	} else {
		// 获取默认主机API
		api, err := portaudio.DefaultHostApi()
		if err == nil && api.DefaultOutputDevice != nil {
			// 使用当前主机API的默认输出设备
			p.device = api.DefaultOutputDevice
			logrus.Infof("使用默认输出设备: %s", p.device.Name)

			outputParams = portaudio.StreamParameters{
				Output: portaudio.StreamDeviceParameters{
					Device:   p.device,
					Channels: p.channelCount,
					Latency:  p.device.DefaultLowOutputLatency,
				},
				SampleRate:      float64(p.sampleRate),
				FramesPerBuffer: p.framesPerBuffer,
			}
		} else {
			// 退化方式：不指定具体设备，让PortAudio选择
			outputParams = portaudio.StreamParameters{
				Output: portaudio.StreamDeviceParameters{
					Device:   nil,
					Channels: p.channelCount,
					Latency:  0,
				},
				SampleRate:      float64(p.sampleRate),
				FramesPerBuffer: p.framesPerBuffer,
			}
		}
	}

	// 播放回调函数
	callback := func(_, out []int16) {
		// 使用延迟恢复机制防止崩溃
		defer func() {
			if rec := recover(); rec != nil {
				logrus.Errorf("播放回调发生异常: %v", rec)
			}
		}()

		p.queueMutex.Lock()
		defer p.queueMutex.Unlock()

		// 如果队列为空，输出静音
		if len(p.queue) == 0 {
			for i := range out {
				out[i] = 0
			}
			return
		}

		// 从队列取出一帧音频数据
		pcmData := p.queue[0]
		p.queue = p.queue[1:] // 移除已处理的数据

		// 复制数据到输出缓冲区
		copyLen := min(len(pcmData), len(out))
		copy(out[:copyLen], pcmData[:copyLen])

		// 如果PCM数据长度小于输出缓冲区，填充静音
		for i := copyLen; i < len(out); i++ {
			out[i] = 0
		}
	}

	// 打开音频流
	stream, err := portaudio.OpenStream(outputParams, callback)
	if err != nil {
		logrus.Errorf("打开音频流失败，尝试PulseAudio: %v", err)

		// 尝试查找PulseAudio设备
		devices, _ := portaudio.Devices()
		var pulseDevice *portaudio.DeviceInfo

		for _, dev := range devices {
			if dev.MaxOutputChannels > 0 && contains(dev.Name, "pulse") {
				pulseDevice = dev
				logrus.Infof("找到PulseAudio输出设备: %s", dev.Name)
				break
			}
		}

		if pulseDevice != nil {
			// 使用PulseAudio设备重试
			outputParams.Output.Device = pulseDevice
			p.device = pulseDevice

			stream, err = portaudio.OpenStream(outputParams, callback)
			if err != nil {
				// 仍然失败，切换到哑模式
				logrus.Warnf("使用PulseAudio打开音频流失败: %v, 将以哑模式运行", err)
				p.dummyMode = true
				p.isPlaying = true
				go p.processQueue()
				return nil
			}
		} else {
			// 未找到PulseAudio设备，切换到哑模式
			logrus.Warnf("打开音频流失败，且未找到PulseAudio设备: %v, 将以哑模式运行", err)
			p.dummyMode = true
			p.isPlaying = true
			go p.processQueue()
			return nil
		}
	}

	// 启动音频流
	if err := stream.Start(); err != nil {
		stream.Close()
		logrus.Warnf("启动音频流失败: %v, 将以哑模式运行", err)
		p.dummyMode = true
		p.isPlaying = true
		go p.processQueue()
		return nil
	}

	p.stream = stream
	p.isPlaying = true
	logrus.Info("音频播放器已启动")

	// 启动处理队列的协程
	go p.processQueue()

	return nil
}

// Stop 停止播放
func (p *AudioPlayerNew) Stop() error {
	p.mutex.Lock()

	// 如果已经停止，直接返回
	if !p.isPlaying {
		p.mutex.Unlock()
		return nil
	}
	p.isPlaying = false
	p.mutex.Unlock()

	// 发送停止信号，防止重复关闭
	p.stopChanMutex.Lock()
	if !p.stopChanClosed {
		close(p.stopChan)
		p.stopChanClosed = true
	}
	p.stopChanMutex.Unlock()

	// 清空队列
	p.queueMutex.Lock()
	p.queue = nil
	p.queueMutex.Unlock()

	// 如果是哑模式，直接返回
	if p.dummyMode {
		return nil
	}

	// 停止并关闭音频流
	return p.stopStreamSafely()
}

// 安全地停止音频流，处理超时情况
func (p *AudioPlayerNew) stopStreamSafely() error {
	// 创建一个通道来接收结果
	done := make(chan error, 1)

	// 在一个独立的goroutine中执行停止操作
	go func() {
		if p.stream == nil {
			done <- nil
			return
		}

		// 先停止流
		err := p.stream.Stop()
		if err != nil {
			done <- fmt.Errorf("停止音频流失败: %v", err)
			return
		}

		// 关闭流
		err = p.stream.Close()
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
	case <-time.After(2 * time.Second):
		logrus.Warn("停止音频流操作超时")
		return fmt.Errorf("停止音频流操作超时")
	}
}

// QueueAudio 将音频数据添加到播放队列
func (p *AudioPlayerNew) QueueAudio(encodedData []byte) {
	if p.decoder == nil || len(encodedData) == 0 {
		return
	}

	// 解码数据
	pcmBuffer := make([]int16, p.framesPerBuffer*p.channelCount) // 足够大的缓冲区
	n, err := p.decoder.Decode(encodedData, pcmBuffer)
	if err != nil {
		logrus.Errorf("解码音频数据失败: %v", err)
		return
	}

	// 只保留有效的PCM数据
	pcmData := make([]int16, n)
	copy(pcmData, pcmBuffer[:n])

	// 添加到队列
	p.queueMutex.Lock()
	defer p.queueMutex.Unlock()
	p.queue = append(p.queue, pcmData)
}

// QueuePCMAudio 将PCM音频数据直接添加到播放队列
func (p *AudioPlayerNew) QueuePCMAudio(pcmData []int16) {
	if len(pcmData) == 0 {
		return
	}

	// 复制数据以避免竞争条件
	dataCopy := make([]int16, len(pcmData))
	copy(dataCopy, pcmData)

	// 添加到队列
	p.queueMutex.Lock()
	defer p.queueMutex.Unlock()
	p.queue = append(p.queue, dataCopy)
}

// processQueue 处理音频队列
func (p *AudioPlayerNew) processQueue() {
	// 创建一个新的停止通道，避免使用已关闭的通道
	stopChan := p.stopChan

	// 添加恢复机制
	defer func() {
		if rec := recover(); rec != nil {
			logrus.Errorf("音频处理协程崩溃: %v", rec)
		}
	}()

	// 哑模式下，每隔一段时间清理队列，模拟播放
	if p.dummyMode {
		timeout := time.NewTicker(time.Duration(p.framesPerBuffer) * time.Second / time.Duration(p.sampleRate))
		defer timeout.Stop()

		for {
			select {
			case <-stopChan:
				return
			case <-timeout.C:
				p.queueMutex.Lock()
				if len(p.queue) > 0 {
					p.queue = p.queue[1:] // 移除一帧数据
				}
				p.queueMutex.Unlock()
			}
		}
	}

	// 在非哑模式下，PortAudio的回调会处理队列中的数据
	// 这里只需等待停止信号
	<-stopChan
}

// IsPlaying 检查是否正在播放
func (p *AudioPlayerNew) IsPlaying() bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.isPlaying
}

// IsDummyMode 检查是否在哑模式下运行
func (p *AudioPlayerNew) IsDummyMode() bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.dummyMode
}

// GetQueueLength 获取当前队列长度
func (p *AudioPlayerNew) GetQueueLength() int {
	p.queueMutex.Lock()
	defer p.queueMutex.Unlock()
	return len(p.queue)
}

// Close 关闭播放器并释放资源
func (p *AudioPlayerNew) Close() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// 添加恢复机制
	defer func() {
		if rec := recover(); rec != nil {
			logrus.Errorf("关闭播放器时发生异常: %v", rec)
		}
	}()

	// 如果正在播放，先停止
	if p.isPlaying {
		p.mutex.Unlock()
		_ = p.Stop() // Stop会处理锁
		p.mutex.Lock()
	}

	// 清除队列和其他引用
	p.queueMutex.Lock()
	p.queue = nil
	p.queueMutex.Unlock()

	p.decoder = nil

	return nil
}

// 工具函数: min返回两个数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
