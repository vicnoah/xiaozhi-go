package audio

import (
	"fmt"
	"sync"
	"time"

	"github.com/hajimehoshi/oto"
	"github.com/sirupsen/logrus"
)

// AudioPlayerNew 音频播放器，使用Oto播放
type AudioPlayerNew struct {
	context         *oto.Context  // Oto上下文
	player          *oto.Player   // Oto播放器
	buffer          []int16       // PCM缓冲区
	mutex           sync.Mutex    // 状态互斥锁
	queue           [][]int16     // PCM数据队列
	queueMutex      sync.Mutex    // 队列互斥锁
	isPlaying       bool          // 是否正在播放
	stopChan        chan struct{} // 停止信号通道
	stopChanMutex   sync.Mutex    // 通道关闭互斥锁
	stopChanClosed  bool          // 通道是否已关闭
	sampleRate      int           // 采样率
	channelCount    int           // 通道数
	framesPerBuffer int           // 每次回调的帧数
	dummyMode       bool          // 哑模式标志
	decoder         Decoder       // 解码器（可选）
}

// NewPlayerOptions 创建播放器的选项
type NewPlayerOptions struct {
	SampleRate       int
	ChannelCount     int
	FramesPerBuffer  int
	UseDefaultDevice bool
	DeviceName       string // 如果不为空，则尝试使用指定名称的设备
}

const maxOpusFrameSize = 5760 // 120ms at 48kHz, 单通道

var otoInited = false

// NewAudioPlayerWithOptions 使用指定选项创建新的音频播放器
func NewAudioPlayerWithOptions(options NewPlayerOptions, decoder Decoder) (*AudioPlayerNew, error) {
	if otoInited {
		return nil, fmt.Errorf("Oto Context 已初始化，不能重复创建")
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

	// 创建Oto上下文
	ctx, err := oto.NewContext(options.SampleRate, options.ChannelCount, 2, options.FramesPerBuffer*options.ChannelCount*2)
	if err != nil {
		return nil, fmt.Errorf("初始化Oto失败: %v", err)
	}
	otoInited = true

	player := &AudioPlayerNew{
		context:         ctx,
		buffer:          make([]int16, options.FramesPerBuffer*options.ChannelCount),
		queue:           make([][]int16, 0, 100),
		stopChan:        make(chan struct{}),
		stopChanMutex:   sync.Mutex{},
		stopChanClosed:  false,
		sampleRate:      options.SampleRate,
		channelCount:    options.ChannelCount,
		framesPerBuffer: options.FramesPerBuffer,
		dummyMode:       false,
		decoder:         decoder,
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

	p.stopChanMutex.Lock()
	if p.stopChanClosed {
		p.stopChan = make(chan struct{})
		p.stopChanClosed = false
	}
	p.stopChanMutex.Unlock()

	if p.isPlaying {
		return nil
	}

	if p.dummyMode {
		p.isPlaying = true
		go p.processQueue()
		return nil
	}

	p.isPlaying = true
	go p.otoPlayLoop()
	return nil
}

// otoPlayLoop 用于持续播放队列中的PCM数据
func (p *AudioPlayerNew) otoPlayLoop() {
	p.player = p.context.NewPlayer()
	defer p.player.Close()
	for {
		select {
		case <-p.stopChan:
			return
		default:
			p.queueMutex.Lock()
			if len(p.queue) == 0 {
				p.queueMutex.Unlock()
				time.Sleep(10 * time.Millisecond)
				continue
			}
			pcmData := p.queue[0]
			p.queue = p.queue[1:]
			p.queueMutex.Unlock()

			// 转换为字节流
			buf := make([]byte, len(pcmData)*2)
			for i, v := range pcmData {
				buf[2*i] = byte(v)
				buf[2*i+1] = byte(v >> 8)
			}
			_, _ = p.player.Write(buf)
		}
	}
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
		if p.player == nil {
			done <- nil
			return
		}

		// 先关闭播放器
		err := p.player.Close()
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
	pcmBuffer := make([]int16, maxOpusFrameSize*p.channelCount) // 足够大的缓冲区
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

// SetDecoder 设置新的解码器
func (p *AudioPlayerNew) SetDecoder(decoder Decoder) {
	p.decoder = decoder
}

// SetAudioParams 同步更新播放器参数
func (p *AudioPlayerNew) SetAudioParams(sampleRate, channelCount, frameDuration int) {
	p.sampleRate = sampleRate
	p.channelCount = channelCount
	p.framesPerBuffer = (sampleRate * frameDuration) / 1000
	p.buffer = make([]int16, p.framesPerBuffer*p.channelCount)
}
