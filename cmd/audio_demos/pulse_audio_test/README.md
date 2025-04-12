# PulseAudio 音频测试工具

这个工具用于测试音频系统的新实现，特别是与PulseAudio的兼容性。它提供了三种运行模式：

1. 打印和查找音频设备 (`pulse`)
2. 生成1KHz正弦波并播放 (`sine`)
3. 录音并每隔2秒播放录制内容 (`record`)

## 主要特点

- 支持指定输入和输出设备
- 自动检测并使用PulseAudio设备
- 支持配置采样率、通道数和帧持续时间
- 直接使用PCM数据流，避免编解码开销
- 对错误恢复和异常处理有更好的支持

## 使用方法

```bash
# 打印音频设备信息，特别关注PulseAudio设备
go run main.go -mode=pulse

# 使用默认设备生成1KHz正弦波
go run main.go -mode=sine

# 指定输出设备生成1KHz正弦波
go run main.go -mode=sine -output="pulse"

# 录音并回放，使用默认设备
go run main.go -mode=record

# 录音并回放，指定输入和输出设备
go run main.go -mode=record -input="pulse" -output="pulse"

# 使用不同的采样率
go run main.go -mode=sine -rate=44100

# 启用详细日志
go run main.go -mode=sine -verbose
```

## 命令行参数

- `-mode`: 运行模式 (pulse, sine, record)
- `-input`: 输入设备名称（部分匹配）
- `-output`: 输出设备名称（部分匹配）
- `-rate`: 采样率（默认16000）
- `-channels`: 通道数（默认1）
- `-duration`: 帧持续时间（毫秒，默认60）
- `-verbose`: 启用详细日志

## 问题排查

如果遇到音频设备访问问题，请尝试以下方法：

1. 确保PulseAudio服务正在运行：`pulseaudio --check`
2. 检查用户是否有权限访问音频设备：`groups | grep audio`
3. 使用`-verbose`参数启用详细日志，查看更多信息
4. 尝试指定设备名称，而不是使用默认设备

## 实现说明

这个测试工具使用了重新实现的音频接口，改进包括：

1. 更好的设备选择机制，支持PulseAudio
2. 增强的错误处理和恢复机制
3. 直接PCM数据流支持，减少编解码开销
4. 更好的资源管理和清理
5. 完整的配置选项 