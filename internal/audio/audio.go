package audio

import (
	"fmt"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/sirupsen/logrus"
)

const (
	DefaultSampleRate      = 16000
	DefaultChannelCount    = 1
	DefaultFrameDuration   = 60  // 毫秒
	DefaultFramesPerBuffer = 960 // 60ms at 16kHz
	DefaultBitrate         = 16000
	DefaultComplexity      = 10
	DefaultMaxValue        = 1<<15 - 1
)

// InitializeAudio 初始化音频系统
func InitializeAudio() error {
	// 初始化PortAudio
	err := portaudio.Initialize()
	if err != nil {
		return err
	}

	logrus.Info("音频系统已初始化")
	return nil
}

// TerminateAudio 终止音频系统
func TerminateAudio() error {
	err := portaudio.Terminate()
	if err != nil {
		logrus.Warnf("终止PortAudio失败: %v", err)
		return err
	}

	logrus.Info("音频系统已终止")
	return nil
}

// GetAudioDevices 获取音频设备列表
func GetAudioDevices() ([]*portaudio.DeviceInfo, error) {
	return portaudio.Devices()
}

// PrintDeviceInfo 打印设备信息
func PrintDeviceInfo() {
	// 获取设备信息
	devices, err := portaudio.Devices()
	if err != nil {
		logrus.Errorf("获取音频设备失败: %v", err)
		return
	}

	// 获取默认主机API
	api, err := portaudio.DefaultHostApi()
	if err == nil {
		logrus.Infof("当前主机API: %s", api.Name)

		// 打印默认设备
		if api.DefaultInputDevice != nil {
			logrus.Infof("默认输入设备: %s", api.DefaultInputDevice.Name)
		} else {
			logrus.Info("没有默认输入设备")
		}

		if api.DefaultOutputDevice != nil {
			logrus.Infof("默认输出设备: %s", api.DefaultOutputDevice.Name)
		} else {
			logrus.Info("没有默认输出设备")
		}
	}

	// 打印所有可用主机API（简化版本，避免使用不兼容的API）
	logrus.Info("可用的音频API:")
	defaultAPI, _ := portaudio.DefaultHostApi()
	if defaultAPI != nil {
		logrus.Infof("默认API: %s", defaultAPI.Name)
	}

	logrus.Info("音频设备列表:")
	for i, dev := range devices {
		logrus.Infof("[%d] 名称: %s", i, dev.Name)
		logrus.Infof("    - 输入通道数: %d", dev.MaxInputChannels)
		logrus.Infof("    - 输出通道数: %d", dev.MaxOutputChannels)
		logrus.Infof("    - 默认采样率: %.0f", dev.DefaultSampleRate)
	}
}

// AudioManagerNew 使用新实现的音频管理器
type AudioManagerNew struct {
	recorder      *RecorderNew    // 新的录音器
	player        *AudioPlayerNew // 新的播放器
	codec         *OpusCodec      // 编解码器
	initialized   bool            // 初始化标志
	sampleRate    int             // 采样率
	channelCount  int             // 通道数
	frameDuration int             // 帧持续时间（毫秒）
}

// AudioManagerOptions 音频管理器选项
type AudioManagerOptions struct {
	SampleRate        int    // 采样率
	ChannelCount      int    // 通道数
	FrameDuration     int    // 帧持续时间（毫秒）
	InputDeviceName   string // 输入设备名称（可选）
	OutputDeviceName  string // 输出设备名称（可选）
	UseDefaultDevices bool   // 是否使用默认设备
}

// NewAudioManagerWithOptions 使用指定选项创建新的音频管理器
func NewAudioManagerWithOptions(options AudioManagerOptions) (*AudioManagerNew, error) {
	// 初始化PortAudio
	err := InitializeAudio()
	if err != nil {
		return nil, fmt.Errorf("初始化音频系统失败: %v", err)
	}

	// 使用默认值处理未指定的选项
	if options.SampleRate <= 0 {
		options.SampleRate = DefaultSampleRate
	}
	if options.ChannelCount <= 0 {
		options.ChannelCount = DefaultChannelCount
	}
	if options.FrameDuration <= 0 {
		options.FrameDuration = DefaultFrameDuration
	}

	// 创建编解码器
	codec, err := NewOpusCodec(options.SampleRate, options.ChannelCount)
	if err != nil {
		TerminateAudio()
		return nil, fmt.Errorf("创建Opus编解码器失败: %v", err)
	}

	// 创建录音器
	recorderOptions := NewRecorderOptions{
		SampleRate:       options.SampleRate,
		ChannelCount:     options.ChannelCount,
		FramesPerBuffer:  (options.SampleRate * options.FrameDuration) / 1000,
		UseDefaultDevice: options.UseDefaultDevices,
		DeviceName:       options.InputDeviceName,
	}

	recorder, err := NewRecorderWithOptions(recorderOptions)
	if err != nil {
		codec.Close()
		TerminateAudio()
		return nil, fmt.Errorf("创建录音器失败: %v", err)
	}

	// 创建播放器
	playerOptions := NewPlayerOptions{
		SampleRate:       options.SampleRate,
		ChannelCount:     options.ChannelCount,
		FramesPerBuffer:  (options.SampleRate * options.FrameDuration) / 1000,
		UseDefaultDevice: options.UseDefaultDevices,
		DeviceName:       options.OutputDeviceName,
	}

	player, err := NewAudioPlayerWithOptions(playerOptions, codec)
	if err != nil {
		recorder.Close()
		codec.Close()
		TerminateAudio()
		return nil, fmt.Errorf("创建播放器失败: %v", err)
	}

	return &AudioManagerNew{
		recorder:      recorder,
		player:        player,
		codec:         codec,
		initialized:   true,
		sampleRate:    options.SampleRate,
		channelCount:  options.ChannelCount,
		frameDuration: options.FrameDuration,
	}, nil
}

// NewAudioManager2 创建新的音频管理器（使用默认选项）
func NewAudioManager2() (*AudioManagerNew, error) {
	options := AudioManagerOptions{
		SampleRate:        DefaultSampleRate,
		ChannelCount:      DefaultChannelCount,
		FrameDuration:     DefaultFrameDuration,
		UseDefaultDevices: true,
	}

	return NewAudioManagerWithOptions(options)
}

// Close 关闭音频管理器并释放资源
func (m *AudioManagerNew) Close() error {
	// 添加一个恢复机制，防止任何异常导致无法正常清理资源
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("关闭音频管理器时发生异常: %v", r)
		}
	}()

	if !m.initialized {
		return nil
	}

	// 关闭录音器
	if m.recorder != nil {
		if err := m.recorder.Close(); err != nil {
			logrus.Warnf("关闭录音器失败: %v", err)
		}
	}

	// 关闭播放器
	if m.player != nil {
		if err := m.player.Close(); err != nil {
			logrus.Warnf("关闭播放器失败: %v", err)
		}
	}

	// 关闭编解码器
	if m.codec != nil {
		m.codec.Close()
	}

	// 等待一小段时间，确保所有资源都释放
	time.Sleep(100 * time.Millisecond)

	// 终止PortAudio
	err := TerminateAudio()
	if err != nil {
		logrus.Warnf("终止音频系统失败: %v", err)
	}

	m.initialized = false
	logrus.Debug("音频管理器已关闭")
	return nil
}

// SetAudioDataCallback 设置音频数据回调函数
func (m *AudioManagerNew) SetAudioDataCallback(callback func([]byte)) {
	m.recorder.SetAudioDataCallback(callback)
}

// SetPCMDataCallback 设置PCM音频数据回调函数
func (m *AudioManagerNew) SetPCMDataCallback(callback func([]int16, int)) {
	m.recorder.SetPCMDataCallback(callback)
}

// StartRecording 开始录音
func (m *AudioManagerNew) StartRecording() error {
	return m.recorder.StartRecording(m.codec)
}

// StopRecording 停止录音
func (m *AudioManagerNew) StopRecording() error {
	return m.recorder.StopRecording()
}

// StartPlaying 开始播放
func (m *AudioManagerNew) StartPlaying() error {
	return m.player.Start()
}

// StopPlaying 停止播放
func (m *AudioManagerNew) StopPlaying() error {
	return m.player.Stop()
}

// PlayAudio 播放Opus编码的音频数据
func (m *AudioManagerNew) PlayAudio(opusData []byte) {
	m.player.QueueAudio(opusData)
}

// PlayPCMAudio 播放PCM音频数据
func (m *AudioManagerNew) PlayPCMAudio(pcmData []int16) {
	m.player.QueuePCMAudio(pcmData)
}

// IsRecording 检查是否正在录音
func (m *AudioManagerNew) IsRecording() bool {
	return m.recorder.IsRecording()
}

// IsPlaying 检查是否正在播放
func (m *AudioManagerNew) IsPlaying() bool {
	return m.player.IsPlaying()
}

// IsDummyMode 检查播放器是否在哑模式下运行
func (m *AudioManagerNew) IsDummyMode() bool {
	return m.player.IsDummyMode()
}

// GetQueueLength 获取播放队列长度
func (m *AudioManagerNew) GetQueueLength() int {
	return m.player.GetQueueLength()
}

// SampleRate 获取采样率
func (m *AudioManagerNew) SampleRate() int {
	return m.sampleRate
}

// ChannelCount 获取通道数
func (m *AudioManagerNew) ChannelCount() int {
	return m.channelCount
}

// FrameDuration 获取帧持续时间（毫秒）
func (m *AudioManagerNew) FrameDuration() int {
	return m.frameDuration
}
