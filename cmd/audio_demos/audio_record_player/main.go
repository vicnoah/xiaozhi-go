package main

import (
	"time"

	"github.com/justa-cai/xiaozhi-go/internal/audio"
	"github.com/sirupsen/logrus"
)

func main() {
	// 创建音频管理器（使用默认参数）
	manager, err := audio.NewAudioManager()
	if err != nil {
		logrus.Fatalf("创建音频管理器失败: %v", err)
	}
	defer manager.Close()

	// 用于存储录音的PCM数据
	pcmBuffer := make([][]int16, 0, 100)

	// 设置PCM数据回调
	manager.SetPCMDataCallback(func(data []int16, size int) {
		dataCopy := make([]int16, size)
		copy(dataCopy, data[:size])
		pcmBuffer = append(pcmBuffer, dataCopy)
	})

	// 开始播放（播放器需先启动）
	if err := manager.StartPlaying(); err != nil {
		logrus.Fatalf("启动音频播放器失败: %v", err)
	}

	// 开始录音
	logrus.Info("开始录音3秒...")
	if err := manager.StartRecording(); err != nil {
		logrus.Fatalf("启动录音失败: %v", err)
	}

	time.Sleep(3 * time.Second)

	// 停止录音
	if err := manager.StopRecording(); err != nil {
		logrus.Errorf("停止录音失败: %v", err)
	}
	logrus.Infof("录音结束，开始播放录音内容，共%d帧", len(pcmBuffer))

	// 播放录音内容
	for _, frame := range pcmBuffer {
		manager.PlayPCMAudio(frame)
	}

	// 播放完后等待3秒，确保音频播放完成
	time.Sleep(3 * time.Second)

	logrus.Info("播放结束，程序退出")
}
