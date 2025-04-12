package main

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/JustaCai/xiaozhi-go/internal/audio"
	"github.com/JustaCai/xiaozhi-go/internal/client"
	"github.com/JustaCai/xiaozhi-go/internal/ota"
	"github.com/JustaCai/xiaozhi-go/internal/protocol"
	"github.com/sirupsen/logrus"
)

// 常量
const (
	StateIdle      = "idle"
	StateListening = "listening"
	StateSpeaking  = "speaking"
)

var (
	// 命令行参数
	serverURL     string
	deviceID      string
	token         string
	boardType     string
	appVersion    string
	activateOnly  bool
	logLevel      string
	skipTLSVerify bool
	httpProxy     string
	// 添加调试标志
	debugEnabled bool
	// 添加详细日志标志
	verboseLogging bool
)

// 全局音频管理器
var (
	audioManager *audio.AudioManagerNew
	audioPlayer  *audio.AudioPlayerNew
)

// 定义一个全局变量，用于追踪是否已恢复终端设置
var terminalRestored bool = false
var terminalMutex sync.Mutex

// 全局音频数据通道
var audioChan chan []byte

func init() {
	// 解析命令行参数
	flag.StringVar(&serverURL, "server", protocol.DefaultWebSocketURL, "WebSocket服务器地址")
	flag.StringVar(&deviceID, "device-id", "", "设备ID (MAC地址)")
	flag.StringVar(&token, "token", "test-token", "API访问令牌")
	flag.StringVar(&boardType, "board", "generic", "设备板型号")
	flag.StringVar(&appVersion, "version", "1.0.0", "应用版本号")
	flag.BoolVar(&activateOnly, "activate-only", false, "只执行激活流程")
	flag.StringVar(&logLevel, "log-level", "info", "日志级别 (debug, info, warn, error, fatal, panic)")
	flag.BoolVar(&skipTLSVerify, "skip-tls-verify", true, "跳过TLS证书验证")
	flag.StringVar(&httpProxy, "http-proxy", "", "HTTP代理地址，例如: http://127.0.0.1:8080")
	// 添加调试标志
	flag.BoolVar(&debugEnabled, "debug", false, "启用高级调试功能")
	// 添加详细日志标志
	flag.BoolVar(&verboseLogging, "verbose", false, "启用详细日志")

	// 配置日志
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	// 默认使用debug级别
	logrus.SetLevel(logrus.InfoLevel)

	// 添加一个日志钩子，以便跟踪WebSocket连接过程
	logrus.AddHook(&WebSocketLogHook{})
}

// WebSocketLogHook 是一个简单的日志钩子，用于跟踪WebSocket连接
type WebSocketLogHook struct{}

// Levels 指定此钩子将处理的日志级别
func (hook *WebSocketLogHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.DebugLevel,
		logrus.InfoLevel,
		logrus.WarnLevel,
		logrus.ErrorLevel,
	}
}

// Fire 处理日志条目
func (hook *WebSocketLogHook) Fire(entry *logrus.Entry) error {
	// 只保留关键连接信息的详细日志，避免重复输出普通日志
	msg := entry.Message
	if (strings.Contains(msg, "WebSocket连接成功") ||
		strings.Contains(msg, "连接失败") ||
		strings.Contains(msg, "hello消息") ||
		strings.Contains(msg, "断开连接")) &&
		entry.Level <= logrus.InfoLevel {
		// 将WebSocket连接关键消息保存到日志文件或特殊格式输出
		fmt.Printf("[WS-CONNECTION] %s: %s\n",
			entry.Time.Format("15:04:05.000"),
			entry.Message)
	}
	return nil
}

// safeExit 安全退出程序，确保恢复终端设置
func safeExit(code int) {
	terminalMutex.Lock()
	defer terminalMutex.Unlock()

	if !terminalRestored {
		// 恢复终端设置
		if err := exec.Command("stty", "-F", "/dev/tty", "echo").Run(); err != nil {
			logrus.Errorf("退出时恢复终端回显失败: %v", err)
		}
		if err := exec.Command("stty", "-F", "/dev/tty", "-cbreak").Run(); err != nil {
			logrus.Errorf("退出时恢复终端规范模式失败: %v", err)
		}
		terminalRestored = true
		logrus.Debug("退出前已恢复终端设置")
	}

	os.Exit(code)
}

// cleanupAndExit 清理资源并安全退出
func cleanupAndExit(c *client.Client, code int) {
	// 直接强制退出，不等待资源清理
	// 设置一个非常短的超时时间
	forcedExit := make(chan struct{})
	go func() {
		select {
		case <-forcedExit:
			return
		case <-time.After(1 * time.Second):
			logrus.Warn("强制结束进程")
			safeExit(1)
		}
	}()

	// 快速清理核心资源
	logrus.Debug("开始快速清理资源...")

	// 使用goroutine并行处理所有清理工作
	var wg sync.WaitGroup

	// 关闭客户端连接 - 最优先处理
	if c != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()

			cleanDone := make(chan struct{})
			go func() {
				logrus.Debug("正在关闭音频通道...")
				// 直接强制关闭连接，不调用客户端的方法
				if proto := c.GetProtocol(); proto != nil {
					if wp, ok := proto.(*protocol.WebsocketProtocol); ok {
						wp.ForceDisconnect()
					} else {
						// 普通关闭
						c.CloseAudioChannel()
					}
				}
				close(cleanDone)
			}()

			// 最多等待200ms
			select {
			case <-cleanDone:
				logrus.Debug("音频通道已关闭")
			case <-time.After(200 * time.Millisecond):
				logrus.Warn("关闭音频通道超时")
			}
		}()
	}

	// 等待所有清理工作完成或超时
	waitChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitChan)
	}()

	// 最多等待500ms
	select {
	case <-waitChan:
		// 所有工作完成
	case <-time.After(500 * time.Millisecond):
		logrus.Warn("资源清理超时")
	}

	// 关闭强制退出
	close(forcedExit)

	// 立即退出
	logrus.Info("正在退出程序")
	safeExit(code)
}

// analyzeConnectionError 分析连接错误
func analyzeConnectionError(err error) {
	logrus.Error("连接错误详细分析:")

	if os.IsTimeout(err) {
		logrus.Error("- 错误类型: 连接超时")
		logrus.Error("- 可能原因: 网络延迟高、服务器无响应或防火墙阻止")
		logrus.Error("- 建议解决方案: 检查网络连接、确认服务器地址正确、检查防火墙设置")
	} else if strings.Contains(err.Error(), "certificate") {
		logrus.Error("- 错误类型: 证书验证错误")
		logrus.Error("- 可能原因: 自签名证书或证书无效")
		logrus.Error("- 建议解决方案: 使用 --skip-tls-verify 选项跳过证书验证")
	} else if strings.Contains(err.Error(), "dial") {
		logrus.Error("- 错误类型: 网络连接错误")
		logrus.Error("- 可能原因: 网络不可达、端口关闭或主机不存在")
		logrus.Error("- 建议解决方案: 确认服务器地址和端口正确、检查网络配置")
	} else if strings.Contains(err.Error(), "proxy") {
		logrus.Error("- 错误类型: 代理连接错误")
		logrus.Error("- 可能原因: 代理配置错误或代理服务不可用")
		logrus.Error("- 建议解决方案: 检查代理配置或暂时禁用代理")
	} else {
		logrus.Error("- 错误类型: 未知错误")
		logrus.Error("- 错误详情:", err.Error())
		logrus.Error("- 建议解决方案: 检查网络环境和服务器状态")
	}
}

func main() {
	flag.Parse()

	// 根据命令行参数设置日志级别
	switch strings.ToLower(logLevel) {
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
	case "info":
		logrus.SetLevel(logrus.InfoLevel)
	case "warn":
		logrus.SetLevel(logrus.WarnLevel)
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
	case "fatal":
		logrus.SetLevel(logrus.FatalLevel)
	case "panic":
		logrus.SetLevel(logrus.PanicLevel)
	default:
		logrus.Warnf("未知的日志级别: %s，使用默认级别 debug", logLevel)
		logrus.SetLevel(logrus.InfoLevel)
	}

	// 在程序退出时确保恢复终端设置
	defer func() {
		exec.Command("stty", "-F", "/dev/tty", "echo").Run()
		exec.Command("stty", "-F", "/dev/tty", "-cbreak").Run()
		logrus.Debug("已恢复终端设置")
	}()

	logrus.Info("正在启动小智客户端...")

	// 获取设备ID
	if deviceID == "" {
		var err error
		deviceID, err = getMACAddress()
		if err != nil {
			logrus.Warnf("无法获取MAC地址: %v", err)
			deviceID = fmt.Sprintf("device-%d", time.Now().Unix())
			logrus.Infof("生成临时设备ID: %s", deviceID)
		}
	}
	logrus.Infof("使用设备ID: %s", deviceID)

	// 如果只执行激活流程
	if activateOnly {
		runActivation()
		return
	}

	// 如果设备未激活，则返回
	if !isDeviceActivated() {
		logrus.Error("设备未激活，请先激活设备")
		return
	}

	// 初始化音频系统
	initAudio()
	defer cleanupAudio()

	// 创建WebSocket协议实例
	proto := protocol.NewWebsocketProtocol()

	// 设置跳过TLS证书验证
	proto.SetSkipTLSVerify(skipTLSVerify)
	if skipTLSVerify {
		logrus.Info("已设置跳过TLS证书验证")
	} else {
		logrus.Info("将验证TLS证书")
	}

	// 创建客户端
	c := client.New(proto)
	c.SetDeviceID(deviceID)

	// 使用基于设备ID生成的UUID作为客户端ID
	clientID := generateUUID(deviceID)
	c.SetClientID(clientID)
	logrus.Infof("使用客户端ID: %s", clientID)

	if token != "" {
		c.SetToken(token)
	}

	// 捕获中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 确保信号处理不会被阻塞
	go func() {
		sig := <-sigChan
		logrus.Infof("接收到信号: %v, 立即退出...", sig)

		// 使用cleanupAndExit功能进行资源清理和安全退出
		cleanupAndExit(c, 0)
	}()

	// 设置回调
	setupCallbacks(c)

	// 设置连接回调
	proto.SetOnConnected(func() {
		logrus.Info("✅ WebSocket连接成功!")

		// 发送hello消息
		helloMsg := map[string]interface{}{
			"type":      "hello",
			"version":   1,
			"transport": "websocket",
			"audio_params": map[string]interface{}{
				"format":         "opus",
				"sample_rate":    16000,
				"channels":       1,
				"frame_duration": 60,
			},
		}

		if err := proto.SendJSON(helloMsg); err != nil {
			logrus.Errorf("❌ 发送hello消息失败: %v", err)
		} else {
			logrus.Info("✅ hello消息发送成功")
		}
	})

	proto.SetOnDisconnected(func(err error) {
		if err != nil {
			logrus.Errorf("❌ WebSocket断开连接: %v", err)

			// 延迟1秒后尝试重连
			go func() {
				logrus.Info("准备在1秒后尝试重新连接...")
				time.Sleep(1 * time.Second)

				logrus.Info("正在尝试重新连接...")
				// 设置请求头
				proto.SetHeader("Authorization", fmt.Sprintf("Bearer %s", token))
				proto.SetHeader("Protocol-Version", "1")
				proto.SetHeader("Device-Id", deviceID)
				proto.SetHeader("Client-Id", generateUUID(deviceID))

				// 连接
				if err := proto.Connect(serverURL); err != nil {
					logrus.Errorf("重新连接失败: %v", err)
					analyzeConnectionError(err)
				} else {
					logrus.Info("✅ 重新连接成功")
				}
			}()
		} else {
			logrus.Info("WebSocket正常断开连接")
		}
	})

	// 设置JSON消息回调
	proto.SetOnJSONMessage(func(data []byte) {
		// 尝试解析JSON格式以便美观打印
		var jsonData interface{}
		if err := json.Unmarshal(data, &jsonData); err == nil {
			if verboseLogging {
				jsonBytes, _ := json.MarshalIndent(jsonData, "", "  ")
				logrus.Infof("📥 接收到JSON数据: \n%s", string(jsonBytes))
			} else {
				// 简化输出，只显示消息类型
				if typeMap, ok := jsonData.(map[string]interface{}); ok {
					if msgType, exists := typeMap["type"]; exists {
						jsonBytes, _ := json.MarshalIndent(jsonData, "", "  ")
						logrus.Infof("📥 接收到消息类型: %v %s", msgType, string(jsonBytes))

						// 处理服务器的hello消息
						if msgType == "hello" {
							// 检查是否包含音频参数
							if audioParams, ok := typeMap["audio_params"].(map[string]interface{}); ok {
								logrus.Info("收到服务器hello消息，包含音频参数")
								// 提取音频参数
								sampleRate, _ := audioParams["sample_rate"].(float64)
								channels, _ := audioParams["channels"].(float64)
								frameDuration, _ := audioParams["frame_duration"].(float64)
								format, _ := audioParams["format"].(string)

								// 验证音频参数有效性
								if sampleRate > 0 && channels > 0 && frameDuration > 0 && format != "" {
									logrus.Infof("重新初始化解码器: format=%s, sample_rate=%v, channels=%v, frame_duration=%v",
										format, sampleRate, channels, frameDuration)
									// 调用重新初始化解码器的函数
									reinitializeOpusDecoder(int(sampleRate), int(channels), int(frameDuration))
								}
							}
						}
					} else {
						logrus.Info("📥 接收到JSON数据")
					}
				}
			}
		} else {
			logrus.Infof("📥 接收到文本数据: %s", string(data))
		}
	})

	// 设置二进制消息回调
	proto.SetOnBinaryMessage(func(data []byte) {
		if verboseLogging {
			logrus.Infof("📥 接收到二进制数据: %d字节", len(data))
		}

		// 处理Opus编码的音频数据
		if audioPlayer != nil {
			// 检查音频播放器状态
			if !audioPlayer.IsPlaying() {
				// 播放器未运行，可能是因为刚初始化或之前有错误
				logrus.Debug("音频播放器未运行，尝试启动...")
				if err := audioPlayer.Start(); err != nil {
					logrus.Errorf("启动音频播放器失败: %v", err)
				}
			}

			// 如果播放器在哑模式下运行，记录一下
			if audioPlayer.IsDummyMode() && verboseLogging {
				logrus.Debug("音频播放器在哑模式下运行，可能无法实际播放音频")
			}

			// 将Opus编码的音频数据添加到播放队列
			audioPlayer.QueueAudio(data)

			if verboseLogging {
				logrus.Debugf("已将%d字节Opus编码音频数据添加到播放队列", len(data))
			}
		} else {
			logrus.Warn("音频播放器未初始化，无法播放收到的音频数据")
			// 可能需要尝试重新初始化播放器
			if audioManager != nil {
				// 尝试使用默认设置重新初始化播放器
				codec, err := audio.NewOpusCodec(16000, 1)
				if err != nil {
					logrus.Errorf("重新初始化音频编解码器失败: %v", err)
				} else {
					audioPlayer = audio.NewAudioPlayer2(16000, 1, 60, codec)
					logrus.Info("已重新初始化音频播放器")
					if err := audioPlayer.Start(); err != nil {
						logrus.Errorf("启动重新初始化的音频播放器失败: %v", err)
					} else {
						// 现在我们有了播放器，重新尝试添加音频数据
						audioPlayer.QueueAudio(data)
						logrus.Debug("已将音频数据添加到重新初始化的播放器队列")
					}
				}
			}
		}
	})

	// 显示按键操作说明
	fmt.Println("按键操作:")
	fmt.Println("  f - 开始录音")
	fmt.Println("  s - 停止录音")
	fmt.Println("  q - 退出程序")

	// 启动按键监听
	keyPressCh := make(chan string)
	commandCh := make(chan string)
	go readInput(keyPressCh, commandCh)

	// 记录录音状态
	isRecording := false

	// 连接服务器
	logrus.Info("准备连接到服务器...")

	// 添加请求头
	proto.SetHeader("Authorization", fmt.Sprintf("Bearer %s", token))
	proto.SetHeader("Protocol-Version", "1")
	proto.SetHeader("Device-Id", deviceID)
	proto.SetHeader("Client-Id", generateUUID(deviceID))

	// 设置握手超时
	proto.SetHandshakeTimeout(15 * time.Second)

	// 	// 连接
	err := proto.Connect(serverURL)
	// connDone <- err
	// }()
	if err != nil {
		logrus.Errorf("❌ 连接失败: %v", err)
		analyzeConnectionError(err)
		return
	}

	// 创建心跳定时器
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	// 主循环
	for {
		select {
		case cmd := <-commandCh:
			logrus.Debugf("主循环收到命令: %s", cmd)
			// 直接处理简单的退出命令
			if cmd == "quit" || cmd == "q" {
				logrus.Info("收到退出命令，准备退出程序...")
				c.CloseAudioChannel()
				cleanupAndExit(c, 0)
			} else {
				logrus.Warnf("不支持的命令: %s", cmd)
			}

		case key := <-keyPressCh:
			// 处理按键事件
			logrus.Debugf("主循环收到按键事件: %s", key)
			handleKeyPress(c, key, &isRecording)

		case <-pingTicker.C:
			// 发送心跳包，保持连接
			if proto.IsConnected() {
				pingMsg := map[string]interface{}{
					"type": "ping",
					"id":   time.Now().Unix(),
				}

				if err := proto.SendJSON(pingMsg); err != nil {
					logrus.Warnf("发送心跳包失败: %v", err)
				}
			}
		}
	}
}

// safeExecute 安全执行函数，防止阻塞主循环
func safeExecute(fn func(), name string) {
	done := make(chan struct{})

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logrus.Errorf("%s过程中发生异常: %v", name, r)
			}
			close(done)
		}()

		fn()
	}()

	// 不等待执行完成，继续主循环
	// 这只是为了捕获panic并记录日志
}

// handleKeyPress 处理按键事件，抽取为单独函数以便安全执行
func handleKeyPress(c *client.Client, key string, isRecording *bool) {
	if key == "F2_PRESSED" && !*isRecording {
		// 先检查客户端是否已连接到服务器
		if !c.GetProtocol().IsConnected() {
			logrus.Error("客户端未连接到服务器，无法开始录音")
			fmt.Println("⚠️ 未连接到服务器，请先使用/connect命令连接")
			return
		}

		*isRecording = true
		logrus.Info("开始录音...")

		// 检查客户端当前状态
		currentState := c.GetState()
		if currentState == client.StateSpeaking {
			logrus.Info("正在中断AI回复以开始录音...")
			c.SendAbortSpeaking("start_recording")

			// 停止音频播放
			stopAudioPlayback(c)

		}

		if currentState != client.StateListening {
			// 如果客户端不在监听状态，先发送开始监听命令
			// 增加超时保护
			commandDone := make(chan error, 1)
			go func() {
				err := c.SendStartListening(client.ListenModeManual)
				commandDone <- err
			}()

			// 等待命令完成或超时
			var err error
			select {
			case err = <-commandDone:
				// 命令已完成
			case <-time.After(3 * time.Second):
				err = fmt.Errorf("发送开始监听命令超时")
				logrus.Error("发送开始监听命令超时")
			}

			if err != nil {
				logrus.Errorf("发送开始监听命令失败: %v", err)
				*isRecording = false
				fmt.Println("⚠️ 开始录音失败，请检查连接状态")
			} else {
				// 等待客户端状态变为监听状态
				waitStart := time.Now()
				stateChanged := false

				for time.Since(waitStart) < 2*time.Second {
					currentState = c.GetState()
					if currentState == client.StateListening {
						stateChanged = true
						break
					}
					time.Sleep(100 * time.Millisecond)
				}

				// 确认状态是否已变更
				if !stateChanged {
					logrus.Error("等待客户端进入监听状态超时")
					*isRecording = false
					fmt.Println("⚠️ 客户端进入监听状态超时")
				} else {
					// 现在开始录音
					startRecording(c)
				}
			}
		} else {
			// 客户端已经在监听状态，直接开始录音
			startRecording(c)
		}
	} else if key == "F2_RELEASED" {
		// 检查客户端当前状态，如果是Speaking状态，则停止播放
		currentState := c.GetState()
		if currentState == client.StateSpeaking {
			logrus.Info("正在中断AI回复...")
			if err := c.SendAbortSpeaking("stop_speaking"); err != nil {
				logrus.Errorf("发送停止讲话命令失败: %v", err)
			}

			// 停止音频播放
			stopAudioPlayback(c)
		}

		// 无论是否在录音状态，都处理F2_RELEASED事件
		if *isRecording {
			*isRecording = false
			logrus.Info("停止录音...")

			// 检查是否已连接到服务器
			if !c.GetProtocol().IsConnected() {
				logrus.Error("客户端未连接到服务器，但尝试停止录音")
				fmt.Println("⚠️ 连接已断开，无法正常停止录音")

				// 即使未连接，也要尝试停止本地录音设备
				if audioManager != nil {
					if err := audioManager.StopRecording(); err != nil {
						logrus.Errorf("停止录音失败: %v", err)
					}
				}

				// 清理音频通道
				if audioChan != nil {
					time.Sleep(50 * time.Millisecond)
					close(audioChan)
					audioChan = nil
				}
				return
			}

			// 停止录音
			if audioManager != nil {
				if err := audioManager.StopRecording(); err != nil {
					logrus.Errorf("停止录音失败: %v", err)
					fmt.Println("⚠️ 停止录音时出现错误")
				} else {
					logrus.Info("已停止录音")
				}

				// 同时向服务器发送停止监听消息
				if err := c.SendStopListening(); err != nil {
					logrus.Errorf("发送停止监听消息失败: %v", err)
				} else {
					logrus.Info("已向服务器发送停止监听消息")
				}
			}

			// 关闭音频数据通道
			if audioChan != nil {
				time.Sleep(50 * time.Millisecond)
				close(audioChan)
				audioChan = nil
			}
		}
	}
}

// initAudio 初始化音频系统
func initAudio() {
	var err error

	logrus.Debug("开始初始化音频系统...")

	// 创建音频管理器
	audioManager, err = audio.NewAudioManager2()
	if err != nil {
		logrus.Warnf("初始化音频管理器失败: %v，将无法录音", err)
		// 不退出程序，继续运行但不使用音频功能
	} else {
		logrus.Debug("音频管理器初始化成功")
	}

	// 创建Opus编解码器和音频播放器
	codec, err := audio.NewOpusCodec(audio.DefaultSampleRate, audio.DefaultChannelCount)
	if err != nil {
		logrus.Warnf("初始化音频编解码器失败: %v，将无法播放声音", err)
	} else {
		// 创建音频播放器
		audioPlayer = audio.NewAudioPlayer2(audio.DefaultSampleRate, audio.DefaultChannelCount, audio.DefaultFrameDuration, codec)

		// 启动音频播放器
		if err := audioPlayer.Start(); err != nil {
			logrus.Warnf("启动音频播放器失败: %v，将无法播放声音", err)
		} else {
			logrus.Debug("音频播放器启动成功")
		}
	}

	logrus.Info("音频系统初始化完成")
}

// cleanupAudio 清理音频系统资源
func cleanupAudio() {
	if audioPlayer != nil {
		if err := audioPlayer.Close(); err != nil {
			logrus.Errorf("关闭音频播放器失败: %v", err)
		}
	}

	if audioManager != nil {
		if err := audioManager.Close(); err != nil {
			logrus.Errorf("关闭音频管理器失败: %v", err)
		}
	}

	// 关闭音频数据通道
	if audioChan != nil {
		logrus.Debug("关闭音频数据通道...")
		time.Sleep(50 * time.Millisecond)
		close(audioChan)
		audioChan = nil
	}
}

// stopAudioPlayback 停止音频播放
func stopAudioPlayback(c *client.Client) {
	// 先等待500毫秒，给音频播放器一些时间处理缓冲区中的数据
	logrus.Debug("等待500毫秒后停止音频播放...")
	time.Sleep(500 * time.Millisecond)

	// 停止音频播放
	if audioPlayer != nil && audioPlayer.IsPlaying() {
		if err := audioPlayer.Stop(); err != nil {
			logrus.Errorf("停止音频播放失败: %v", err)
		} else {
			logrus.Info("已停止音频播放")
		}
	}
}

// runActivation 运行激活流程
func runActivation() {
	logrus.Info("开始执行设备激活流程...")

	// 创建OTA客户端
	otaClient := ota.NewOTAClient(deviceID, appVersion, boardType)

	// 请求激活
	resp, err := otaClient.RequestActivation()
	if err != nil {
		logrus.Fatalf("设备激活失败: %v", err)
	}

	logrus.Info("设备激活成功!")
	logrus.Infof("激活码: %s", resp.Activation.Code)
	logrus.Infof("固件版本: %s", resp.Firmware.Version)
	logrus.Infof("MQTT配置: 端点=%s, 客户端ID=%s",
		resp.MQTT.Endpoint, resp.MQTT.ClientID)
}

// setupCallbacks 设置客户端回调
func setupCallbacks(c *client.Client) {
	// 状态变更回调
	c.SetOnStateChanged(func(oldState, newState string) {
		logrus.Infof("客户端状态变更: %s -> %s", oldState, newState)

		// 处理不同的状态变更
		if oldState != StateListening && newState == StateListening {
			// 进入监听状态，开始录音
			startRecording(c)
		} else if oldState == StateListening && newState != StateListening {
			// 退出监听状态，停止录音
			stopRecording(c)
		}
	})

	// 网络错误回调
	c.SetOnNetworkError(func(err error) {
		logrus.Errorf("网络错误: %v", err)
	})

	// 识别文本回调
	c.SetOnRecognizedText(func(text string) {
		logrus.Infof("识别到文本: %s", text)
	})

	// 朗读文本回调
	c.SetOnSpeakText(func(text string) {
		logrus.Infof("AI回复: %s", text)
	})

	// 音频数据回调
	c.SetOnAudioData(func(data []byte) {
		logrus.Debugf("收到音频数据: %d字节", len(data))
		// 将音频数据添加到播放队列
		if audioPlayer != nil && audioPlayer.IsPlaying() {
			audioPlayer.QueueAudio(data)
			if audioPlayer.IsDummyMode() {
				// 如果是哑模式，简单记录一下
				logrus.Debugf("音频在哑模式下处理")
			}
		}
	})

	// 情感变更回调
	c.SetOnEmotionChanged(func(emotion, text string) {
		logrus.Infof("情感变更: %s, 表情: %s", emotion, text)
	})

	// IoT命令回调
	c.SetOnIoTCommand(func(commands []interface{}) {
		logrus.Infof("收到IoT命令: %v", commands)
		// 这里可以实现IoT命令处理
	})

	// 音频通道打开回调
	c.SetOnAudioChannelOpen(func() {
		logrus.Info("音频通道已打开")
	})

	// 音频通道关闭回调
	c.SetOnAudioChannelClosed(func() {
		logrus.Info("音频通道已关闭")
		// 如果正在录音，停止录音
		stopRecording(c)
	})
}

// startRecording 开始录音
func startRecording(c *client.Client) {
	logrus.Debug("开始录音流程")
	if audioManager == nil {
		logrus.Error("音频管理器未初始化，无法录音")
		return
	}

	if audioManager.IsRecording() {
		logrus.Debug("已经在录音中，不需要重新开始")
		return
	}

	// 如果客户端不在监听状态，确保先发送开始监听命令
	if c != nil && c.GetState() != client.StateListening {
		if err := c.SendStartListening(client.ListenModeManual); err != nil {
			logrus.Errorf("发送开始监听命令失败: %v", err)
			return
		}
		logrus.Info("已向服务器发送开始监听命令")
	}

	// 如果已有通道在运行，先关闭它
	if audioChan != nil {
		close(audioChan)
		time.Sleep(50 * time.Millisecond)
	}

	// 创建一个带缓冲的通道来接收音频数据
	audioChan = make(chan []byte, 100) // 足够大的缓冲区

	// 启动一个单独的goroutine处理音频数据发送
	go func() {
		for data := range audioChan {
			// 发送音频数据到服务器
			startTime := time.Now()
			err := c.SendAudioData(data)
			elapsed := time.Since(startTime)

			if err != nil {
				logrus.Errorf("发送音频数据失败: %v", err)
			} else if elapsed > 100*time.Millisecond {
				logrus.Warnf("发送音频数据耗时较长: %v，数据大小: %d字节", elapsed, len(data))
			}
		}
		logrus.Debug("音频数据处理已停止")
	}()

	// 设置音频数据回调
	audioManager.SetAudioDataCallback(func(data []byte) {
		// 确保通道未关闭
		if audioChan == nil {
			return
		}

		// 发送到通道，不阻塞
		select {
		case audioChan <- data:
			// 成功发送数据，无需日志
		default:
			// 通道已满，丢弃此数据包
			logrus.Warn("音频数据通道已满，丢弃数据包")
		}
	})

	// 开始录音
	var err error
	err = audioManager.StartRecording()
	if err != nil {
		logrus.Errorf("开始录音失败: %v，将无法发送语音", err)
		if audioChan != nil {
			close(audioChan)
			audioChan = nil
		}
	} else {
		logrus.Info("已成功开始录音")
	}
}

// stopRecording 停止录音并发送停止监听消息到服务器
func stopRecording(c *client.Client) {
	if audioManager == nil {
		return
	}

	// 停止录音
	if err := audioManager.StopRecording(); err != nil {
		logrus.Errorf("停止录音失败: %v", err)
	} else {
		logrus.Info("已停止录音")
	}

	// 向服务器发送停止监听的消息
	if c != nil {
		currentState := c.GetState()
		if currentState == client.StateListening {
			if err := c.SendStopListening(); err != nil {
				logrus.Errorf("发送停止监听消息失败: %v", err)
			} else {
				logrus.Info("已向服务器发送停止监听消息")
			}
		}
	}
}

// generateUUID 基于MAC地址生成UUID
func generateUUID(macAddr string) string {
	// 如果MAC地址为空，使用随机数据
	var data []byte
	if macAddr == "" {
		data = make([]byte, 16)
		rand.Read(data)
	} else {
		// 使用MAC地址作为种子计算MD5
		h := md5.New()
		h.Write([]byte(macAddr))
		data = h.Sum(nil)
	}

	// 设置UUID版本 (版本4)
	data[6] = (data[6] & 0x0F) | 0x40
	// 设置变体
	data[8] = (data[8] & 0x3F) | 0x80

	// 按UUID格式转换为字符串
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		data[0:4], data[4:6], data[6:8], data[8:10], data[10:16])
}

// getMACAddress 获取本机MAC地址
func getMACAddress() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, i := range interfaces {
		if i.Flags&net.FlagUp != 0 && i.Flags&net.FlagLoopback == 0 {
			if len(i.HardwareAddr) > 0 {
				return strings.ToLower(i.HardwareAddr.String()), nil
			}
		}
	}

	return "", fmt.Errorf("未找到有效的网络接口")
}

// isDeviceActivated 检查设备是否已激活
func isDeviceActivated() bool {
	// 创建OTA客户端
	otaClient := ota.NewOTAClient(deviceID, appVersion, boardType)

	// 检查激活状态
	activated, err := otaClient.CheckActivationStatus()
	if err != nil {
		logrus.Errorf("检查设备激活状态失败: %v", err)
		return false
	}

	return activated
}

// readInput 处理按键输入
func readInput(keyPressCh chan<- string, commandCh chan<- string) {
	// 设置终端为原始模式
	if err := exec.Command("stty", "-F", "/dev/tty", "cbreak", "min", "1").Run(); err != nil {
		logrus.Errorf("设置终端cbreak模式失败: %v", err)
	}
	// 关闭终端回显
	if err := exec.Command("stty", "-F", "/dev/tty", "-echo").Run(); err != nil {
		logrus.Errorf("关闭终端回显失败: %v", err)
	}

	// 即使在goroutine中发生panic，也要尝试恢复终端设置
	defer func() {
		if err := exec.Command("stty", "-F", "/dev/tty", "echo").Run(); err != nil {
			logrus.Errorf("恢复终端回显失败: %v", err)
		}
		if err := exec.Command("stty", "-F", "/dev/tty", "-cbreak").Run(); err != nil {
			logrus.Errorf("恢复终端规范模式失败: %v", err)
		}
	}()

	// 记录录音按键状态，防止重复触发
	recordKeyPressed := false

	for {
		var b [1]byte
		_, err := os.Stdin.Read(b[:])
		if err != nil {
			logrus.Errorf("读取输入失败: %v", err)
			continue
		}

		// 处理特殊命令，仅保留退出功能
		if b[0] == 'q' || b[0] == 'Q' {
			// 退出命令
			logrus.Info("准备退出程序")
			commandCh <- "quit"
			continue
		}

		// 处理录音相关按键
		switch b[0] {
		case 'f', 'F': // 按f开始录音
			if !recordKeyPressed {
				recordKeyPressed = true
				keyPressCh <- "F2_PRESSED"
			}
		case 's', 'S': // 按s停止录音
			if recordKeyPressed {
				recordKeyPressed = false
				keyPressCh <- "F2_RELEASED"
			}
		}
	}
}

// reinitializeOpusDecoder 重新初始化Opus解码器
func reinitializeOpusDecoder(sampleRate, channels, frameDuration int) {
	// 忽略无效参数
	if sampleRate <= 0 || channels <= 0 || frameDuration <= 0 {
		logrus.Error("无效的音频参数，无法初始化Opus解码器")
		return
	}

	logrus.Infof("开始重新初始化Opus解码器: sample_rate=%d, channels=%d, frame_duration=%d",
		sampleRate, channels, frameDuration)

	// 如果当前没有audioPlayer，记录错误并返回
	if audioPlayer == nil {
		logrus.Error("audioPlayer未初始化，无法重新初始化解码器")
		return
	}

	// 先停止当前的audioPlayer
	if audioPlayer.IsPlaying() {
		if err := audioPlayer.Stop(); err != nil {
			logrus.Warnf("停止当前音频播放器失败: %v", err)
		}
	}

	// 创建新的audioPlayer，使用更兼容的采样率
	codec, err := audio.NewOpusCodec(sampleRate, channels)
	if err != nil {
		logrus.Errorf("创建新的音频编解码器失败: %v", err)
		return
	}
	newAudioPlayer := audio.NewAudioPlayer2(sampleRate, channels, frameDuration, codec)

	// 关闭旧的audioPlayer
	if err := audioPlayer.Close(); err != nil {
		logrus.Warnf("关闭旧的音频播放器失败: %v", err)
	}

	// 更新全局audioPlayer
	audioPlayer = newAudioPlayer

	// 启动新的audioPlayer
	if err := audioPlayer.Start(); err != nil {
		logrus.Warnf("启动新的音频播放器失败: %v", err)
	} else {
		logrus.Info("✅ 成功重新初始化Opus解码器并启动音频播放器")
	}
}
