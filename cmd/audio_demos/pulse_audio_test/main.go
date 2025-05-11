package main

import (
	"context"
	"flag"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/justa-cai/xiaozhi-go/internal/audio"
	"github.com/sirupsen/logrus"
)

var (
	mode           string
	inputDevice    string
	outputDevice   string
	sampleRate     int
	channelCount   int
	frameDuration  int
	verboseLogging bool
)

func init() {
	// 解析命令行参数
	flag.StringVar(&mode, "mode", "sine", "运行模式: sine=生成1K正弦波, record=录音并播放, pulse=打印PulseAudio设备")
	flag.StringVar(&inputDevice, "input", "", "输入设备名称（部分匹配）")
	flag.StringVar(&outputDevice, "output", "", "输出设备名称（部分匹配）")
	flag.IntVar(&sampleRate, "rate", audio.DefaultSampleRate, "采样率")
	flag.IntVar(&channelCount, "channels", audio.DefaultChannelCount, "通道数")
	flag.IntVar(&frameDuration, "duration", audio.DefaultFrameDuration, "帧持续时间（毫秒）")
	flag.BoolVar(&verboseLogging, "verbose", false, "启用详细日志")
}

func main() {
	flag.Parse()

	// 配置日志级别
	if verboseLogging {
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}

	// 打印音频设备信息
	if mode == "pulse" {
		err := audio.InitializeAudio()
		if err != nil {
			logrus.Fatalf("初始化音频系统失败: %v", err)
		}
		defer audio.TerminateAudio()

		audio.PrintDeviceInfo()
		findPulseAudioDevices()
		return
	}

	// 根据模式执行不同的功能
	switch strings.ToLower(mode) {
	case "sine":
		runSineWaveGenerator()
	case "record":
		runRecordAndPlayback()
	default:
		logrus.Fatalf("未知模式: %s", mode)
	}
}

// 查找PulseAudio设备
func findPulseAudioDevices() {
	devices, err := audio.GetAudioDevices()
	if err != nil {
		logrus.Errorf("获取音频设备失败: %v", err)
		return
	}
	if devices == nil || len(devices) == 0 {
		logrus.Info("当前平台不支持音频设备枚举")
		return
	}

	logrus.Info("查找PulseAudio设备:")
	found := false

	for i, dev := range devices {
		// 这里假设有 DeviceInfo 类型，否则直接跳过
		info, ok := dev.(struct {
			Name              string
			MaxInputChannels  int
			MaxOutputChannels int
		})
		if !ok {
			logrus.Warnf("未知设备类型，跳过: %#v", dev)
			continue
		}
		if strings.Contains(strings.ToLower(info.Name), "pulse") {
			logrus.Infof("[%d] 找到PulseAudio设备: %s", i, info.Name)
			if info.MaxInputChannels > 0 {
				logrus.Infof("    - 可用作输入设备（通道数: %d）", info.MaxInputChannels)
			}
			if info.MaxOutputChannels > 0 {
				logrus.Infof("    - 可用作输出设备（通道数: %d）", info.MaxOutputChannels)
			}
			found = true
		}
	}

	if !found {
		logrus.Info("未找到任何PulseAudio设备")
	}
}

// 运行正弦波生成器
func runSineWaveGenerator() {
	logrus.Info("开始运行1K波形生成器 (新实现)")

	// 创建音频管理器选项
	options := audio.AudioManagerOptions{
		SampleRate:        sampleRate,
		ChannelCount:      channelCount,
		FrameDuration:     frameDuration,
		OutputDeviceName:  outputDevice,
		UseDefaultDevices: outputDevice == "",
	}

	// 创建音频管理器
	manager, err := audio.NewAudioManagerWithOptions(options)
	if err != nil {
		logrus.Fatalf("创建音频管理器失败: %v", err)
	}
	defer manager.Close()

	// 开始播放
	if err := manager.StartPlaying(); err != nil {
		logrus.Fatalf("启动音频播放器失败: %v", err)
	}

	// 创建上下文和取消函数，用于优雅退出
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 处理信号，实现优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logrus.Info("收到终止信号，程序即将退出...")
		cancel()
	}()

	// 启动波形生成器
	go generateSineWave(ctx, manager, sampleRate, frameDuration)

	// 等待上下文结束
	<-ctx.Done()

	// 停止播放
	if err := manager.StopPlaying(); err != nil {
		logrus.Errorf("停止音频播放器失败: %v", err)
	}

	logrus.Info("程序已退出")
}

// 生成正弦波
func generateSineWave(ctx context.Context, manager *audio.AudioManagerNew, sampleRate, frameDuration int) {
	// 1KHz正弦波的周期 = 采样率/频率
	frequency := 1000.0
	sampleRateF := float64(sampleRate)
	period := sampleRateF / frequency

	// 创建缓冲区
	framesPerBuffer := (sampleRate * frameDuration) / 1000
	buffer := make([]int16, framesPerBuffer)

	logrus.Infof("开始生成1KHz正弦波，采样率: %d Hz", sampleRate)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// 生成正弦波数据
			for i := 0; i < len(buffer); i++ {
				// 计算当前时间点对应的角度
				angle := 2.0 * math.Pi * float64(i) / period
				// 生成正弦波，振幅设置为最大值的50%，以避免声音过大
				amplitude := float64(audio.DefaultMaxValue) * 0.5
				buffer[i] = int16(amplitude * math.Sin(angle))
			}

			// 播放PCM数据
			manager.PlayPCMAudio(buffer)

			// 短暂延时，控制生成速率
			time.Sleep(time.Duration(frameDuration) * time.Millisecond / 2)
		}
	}
}

// 运行录音和播放
func runRecordAndPlayback() {
	logrus.Info("开始运行录音-播放演示 (新实现)")

	// 创建音频管理器选项
	options := audio.AudioManagerOptions{
		SampleRate:        sampleRate,
		ChannelCount:      channelCount,
		FrameDuration:     frameDuration,
		InputDeviceName:   inputDevice,
		OutputDeviceName:  outputDevice,
		UseDefaultDevices: inputDevice == "" && outputDevice == "",
	}

	// 创建音频管理器
	manager, err := audio.NewAudioManagerWithOptions(options)
	if err != nil {
		logrus.Fatalf("创建音频管理器失败: %v", err)
	}
	defer manager.Close()

	// 开始播放
	if err := manager.StartPlaying(); err != nil {
		logrus.Fatalf("启动音频播放器失败: %v", err)
	}

	// 创建上下文和取消函数，用于优雅退出
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 处理信号，实现优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logrus.Info("收到终止信号，程序即将退出...")
		cancel()
	}()

	// 启动录音-播放程序
	recordAndPlay(ctx, manager)

	// 停止播放
	if err := manager.StopPlaying(); err != nil {
		logrus.Errorf("停止音频播放器失败: %v", err)
	}

	logrus.Info("程序已退出")
}

// 录音并延迟播放
func recordAndPlay(ctx context.Context, manager *audio.AudioManagerNew) {
	// 设置PCM数据回调
	pcmBuffer := make([][]int16, 0, 100)

	manager.SetPCMDataCallback(func(data []int16, size int) {
		// 复制数据以避免竞争条件
		dataCopy := make([]int16, size)
		copy(dataCopy, data[:size])

		// 添加到缓冲区
		pcmBuffer = append(pcmBuffer, dataCopy)
	})

	// 开始录音
	logrus.Info("开始录音...")
	if err := manager.StartRecording(); err != nil {
		logrus.Errorf("启动录音失败: %v", err)
		return
	}

	// 创建计时器，每2秒播放一次录制的内容
	playbackTicker := time.NewTicker(2 * time.Second)
	defer playbackTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			// 停止录音
			if err := manager.StopRecording(); err != nil {
				logrus.Errorf("停止录音失败: %v", err)
			}
			return
		case <-playbackTicker.C:
			// 获取当前缓冲区中的所有数据
			if len(pcmBuffer) == 0 {
				continue
			}

			// 复制当前缓冲区内容并清空
			currentBuffer := make([][]int16, len(pcmBuffer))
			copy(currentBuffer, pcmBuffer)
			pcmBuffer = nil

			// 播放所有录制的数据
			logrus.Infof("正在播放 %d 帧音频数据...", len(currentBuffer))
			for _, frame := range currentBuffer {
				manager.PlayPCMAudio(frame)
			}
		}
	}
}
